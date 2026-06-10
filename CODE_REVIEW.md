# cs2-server — Comprehensive Code Review

**Date:** 2026-06-10
**Scope:** Entire repository — Go control plane (`cmd/`, `internal/`), C# SwiftlyS2 plugins (`plugins/`), Docker/infra (`docker/`, `deploy/`, `compose.yaml`, `Makefile`).
**Method:** Multi-agent fan-out review (33 agents): ground-truth build/test/lint + repo inventory → 9 parallel domain reviews (security, concurrency, correctness, API/HTTP, reliability/resource, C# plugin, Docker/infra, Go quality, tests) → consolidated synthesis. **94 findings** (4 Critical, 17 High, 26 Medium, 31 Low, 16 Info).

> **Verification caveat:** The automated adversarial-verification phase failed to emit structured output for all 21 Critical/High findings (a workflow/schema hiccup). The synthesizer compensated by **independently re-reading the source** to confirm each Critical/High item; findings therefore carry reviewer + synthesizer confidence, not a separate independent third-pass refutation. Items not re-read are called out under *Coverage Gaps*. This is a **static review** — no runtime/integration testing was performed beyond the build/test gates below.

---

## Verdict

**Engineering hygiene is strong; the control-plane security posture is critical.**

The hard gates are green: `go build`, `go vet`, `gofmt`, and `go test -race` all pass with no hangs; both .NET plugins build in Release with 0 warnings; `gitleaks` finds 0 leaks; `.env` is properly untracked. The defining problem is a **single security chain**: the orchestrator HTTP API has **no authentication**, is **published on all host interfaces** (`:18080`), and the orchestrator runs **as root with the Docker socket mounted**. Anyone who can reach `:18080` gets unauthenticated, host-root-equivalent control. Roughly six separate findings (no-auth, IDOR, quota bypass, DoS, RCON exposure, "the Discord admin gate is not a real boundary") all hang off this one missing boundary.

Three confirmed, player-visible correctness/reliability bugs sit beneath the security layer: a **data race** on the shared `*Instance`, an **idle reaper that destroys healthy servers (and their data volumes)** when RCON is merely unreachable, and a **Duel1v1 double-count** when both players die in the same round. Resource leaks (ports/volumes/records on failed provision, no reconciler) and **near-zero test coverage of the highest-risk packages** (api 0%, reaper 0%, bot authz 0%, docker lifecycle 8%) round out the picture.

---

## Ground-truth gates

| Gate | Result |
|---|---|
| `go build ./...` | ✅ PASS (no output) |
| `go vet ./...` | ✅ PASS (no diagnostics) |
| `go test -race ./...` | ✅ PASS, no hangs (no test files in cmd/bot, cmd/orchestrator, internal/api, internal/apiclient, internal/bot, internal/model, internal/reaper) |
| `gofmt -l .` | ✅ PASS (0 files) |
| `golangci-lint run` | ❌ FAIL — 11 issues (9 errcheck unchecked deferred `Close()`/`ContainerRemove`, 2 staticcheck: QF1012 `handlers.go:96`, SA1019 deprecated `PermissionManageServer` `handlers.go:332`) |
| `dotnet build` (Duel1v1, SamplePlugin, Release) | ✅ PASS — 0 warnings / 0 errors |
| `shellcheck docker/cs2/*.sh` | ⚠️ 1 info-level SC2317 false positive (`install-mods.sh:26`) |
| `hadolint` (game Dockerfile) | ⚠️ 3 warnings (DL3007 `:latest`, DL3008 unpinned apt, DL3003 cd-vs-WORKDIR); deploy Dockerfiles clean |
| `gitleaks detect` | ✅ PASS — 0 leaks, 23 commits |
| Secret scan / `.env` | ✅ `.env` untracked & gitignored; only legitimate field/param/env-name references in tracked source |

---

## Critical findings

### C1 — Orchestrator HTTP API has no authentication on any lifecycle route
**`internal/api/api.go:35-44`; `cmd/orchestrator/main.go:69-74`**
`routes()` wires `POST/GET/DELETE /v1/servers`, `restart`, `stop`, and `stop-all` straight to the manager with no middleware; `main.go` wraps the bare mux with only `ReadHeaderTimeout`. No auth/bearer/token handling exists anywhere in `internal/api` or `cmd/orchestrator`. Anyone who can reach the listener can create unlimited containers, hijack any owner's servers, or `DELETE /v1/servers` with no `owner_id` to mass-kill every server. **Root cause of the IDOR, quota-bypass, and DoS findings.**
**Fix:** Add auth middleware around `s.mux`: require a shared bearer token (constant-time compare) injected via env (`CS2C_API_TOKEN`) and sent by the bot's `apiclient`. Treat `OwnerID` as an authenticated identity, not a client-supplied string.

### C2 — Unauthenticated API published to all host interfaces while orchestrator runs as root with the Docker socket
**`compose.yaml:60` (`18080:8080`), `compose.yaml:39` (`user "0:0"`), `compose.yaml:64` (`/var/run/docker.sock`)**
The API binds `:8080` in-container, published on host `0.0.0.0:18080`; the orchestrator is root with the rw Docker socket (`docker.go` uses `client.FromEnv`). Reaching the unauthenticated API therefore yields full Docker-API control as root ≈ host-root (attacker launches a privileged/host-mounting container). The bot already reaches the orchestrator over the internal `cs2net` at `http://orchestrator:8080`, so the host publish is unnecessary.
**Fix:** Don't publish the API port at all (rely on `cs2net`); if a host port is required, bind `127.0.0.1:18080:8080`. Front the Docker socket with a least-privilege socket proxy and authenticate the API.

> C1 + C2 are effectively one chain and the dominant risk. The remaining two "Critical" tallies are the IDOR (H1 below, rated Critical-adjacent) and the root+SYS_ADMIN container posture (H7) — they collapse once C1/C2 are fixed.

---

## High findings

### H1 — Client-supplied `owner_id` enables IDOR / impersonation and quota bypass
**`internal/api/api.go:48-49,95-105,128-130,180-182`** — `OwnerID` comes verbatim from the request with no binding to the caller; `handleList`/`handleStopAll` trust it, so any caller can enumerate or mass-stop another user's servers. The per-owner cap is keyed on the attacker-chosen `owner_id` and only enforced `if req.OwnerID != ""`, so `owner_id=""` bypasses the cap. **Fix:** derive the principal from authenticated identity (C1); scope List/StopAll to it; reject empty `owner_id` when a cap is set.

### H2 — Data race on shared `*Instance` between `Create`'s response and the detached `provision()` goroutine
**`internal/orchestrator/docker.go:145-147,156,160-161`; `internal/api/api.go:125,68-69`** — `Create` launches `go m.provision(...)` then returns the **same pointer**; `provision` writes `inst.Status`/`inst.BackendID` with no lock while `handleCreate` calls `view(inst)→json.Encode` reading every field. Torn reads, garbage to the client, guaranteed `-race` abort once a test exercises it. **Fix:** `snapshot := *inst; go m.provision(opts, inst, gslt); return &snapshot, nil` (or guard with a mutex / mutate only the DB row).

### H3 — Idle reaper force-stops (and deletes the data volume of) healthy servers when RCON is unreachable
**`internal/orchestrator/docker.go:522-527`; `internal/reaper/reaper.go:69-90`; volume delete `docker.go:459-470`** — `Status` treats an RCON failure as `ls.Online = true; return ls, nil` leaving `HumanCount=0`. The reaper only clears tracking on `err != nil || !st.Online`, so a `true/0` result is **not** cleared, the `HumanCount>0` guard is skipped, and after `idleFor` it calls `Stop` — which removes the container **and its per-instance data volume**. Any running server with wedged RCON (or one full of players whose status read failed) is reaped silently. **Fix:** add `OccupancyKnown bool` to `LiveStatus`, set true only after a successful `ParseStatus`; treat unknown occupancy like `!Online` in the reaper. Add a startup grace window keyed on `CreatedAt`.

### H4 — Per-owner cap is a TOCTOU check the manager never enforces
**`internal/api/api.go:95-105`; `internal/orchestrator/docker.go:96-148`** — cap is read-then-act in the API (List then Create) with no spanning lock; `DockerManager.Create` never re-checks nor returns `model.ErrLimitExceeded` (so the `api.go:214` branch is dead code). N concurrent POSTs for one owner all see `len<maxPer` and proceed, overshooting the quota and exhausting the port range. **Fix:** enforce the cap inside `Create` atomically (count + insert under one tx/mutex) using `store.CountByOwner` (currently dead code); return `ErrLimitExceeded`.

### H5 — Game-server RCON TCP port published to `0.0.0.0` on the host
**`internal/orchestrator/docker.go:229-233`** — every container publishes `tcpRCON → {HostIP: "0.0.0.0", HostPort}`. RCON is a full remote-command channel over plaintext (password sent in clear). The orchestrator reaches RCON over `cs2net` by container name, so the host publish is unnecessary and exposes RCON across the predictable `27015-27115` window. **Fix:** remove `tcpRCON` from `bindings` (keep in `exposed`); if needed for debugging, bind `127.0.0.1` only.

### H6 — No request body size limit or read/write timeouts (memory-exhaustion + Slowloris DoS)
**`internal/api/api.go:79-83`; `cmd/orchestrator/main.go:70-74`** — `handleCreate` does `json.Decode` with no `http.MaxBytesReader`; server sets only `ReadHeaderTimeout`. Unauthenticated, a client can POST a multi-GB body to OOM the orchestrator or trickle bodies to exhaust connections — taking down the control plane and reaper. **Fix:** `r.Body = http.MaxBytesReader(w, r.Body, 64<<10)` before decode; set `ReadTimeout`/`WriteTimeout`/`IdleTimeout` (≈15s/30s/120s) and `MaxHeaderBytes`.

### H7 — Shared-files mode (the default) runs game containers as root with `CAP_SYS_ADMIN` + `apparmor=unconfined`
**`internal/orchestrator/docker.go:266-293`; `compose.yaml:48`; `docker/cs2/mods-entrypoint.sh:78-79`** — when `SharedGameFiles` is on (default), each container gets `User="0:0"`, `CapAdd=["SYS_ADMIN"]`, `SecurityOpt=["apparmor=unconfined"]`, `/dev/fuse`. `CAP_SYS_ADMIN` + unconfined AppArmor is a broad container-escape primitive, on containers running user plugins/workshop assets and (on public servers) facing the internet. **Fix:** prefer rootless/userns + fuse-overlayfs so `SYS_ADMIN` is unnecessary, or pre-seed the merged tree; if unavoidable, add `no-new-privileges`, a tailored seccomp/AppArmor profile (not unconfined), and drop all other caps. Consider defaulting `CS2C_SHARED_GAME_FILES=false` on hardened hosts.

### H8 — Failed provision leaks the per-instance volume, ports, and a zombie store record (no reconciliation)
**`internal/orchestrator/docker.go:151-165,237-240`; `ListManagedContainers` `docker.go:543-556` never called** — `startContainer` creates `cs2-data-<id>` before `ContainerCreate`; on failure only the container is force-removed — the volume is orphaned. `provision`'s failure path sets `StatusError` but never calls `releasePorts` and never deletes the record, so two ports stay reserved for the process lifetime and the dead row consumes the owner's quota. `ListManagedContainers` exists for reconciliation but has no caller. **Fix:** in the failure branch run the same cleanup as `Stop` (`VolumeRemove`, `releasePorts`, delete record); call `ListManagedContainers` at startup / periodic `Reconcile`.

### H9 — `provision()` runs under `context.Background()` and is never drained on shutdown
**`internal/orchestrator/docker.go:152,162`; `cmd/orchestrator/main.go:38,45,63,76-81`** — on SIGINT/SIGTERM the deferred `st.Close()`/`mgr.Close()` run while in-flight provisions continue: a provision can hit `store.Put` on a closed DB, or `ContainerCreate` after the client is closed, leaving instances stuck in `starting` or started-but-unrecorded. `ensureSeeded` also holds `seedMu` across an unbounded `ContainerWait`. **Fix:** thread a manager-lifetime context into `provision`, track goroutines with a `WaitGroup` that `Close()` drains with a deadline, and wrap the seed `ContainerWait` in `context.WithTimeout`.

### H10 — Duel1v1 `OnPlayerDeath` double-counts when both players die in the same round
**`plugins/Duel1v1/Match.cs:210-238` (AddPoint/`_roundsPlayed++` at 230-231)** — per-death Post hook with no round-decided latch. In 1v1 both pawns can die in one round (mutual HE/molotov, simultaneous trade); each death runs `AddPoint(other)+_roundsPlayed++` and schedules `EvaluateAfterRound`, so one round can register two wins and advance `_roundsPlayed` by two — corrupting first-to-N, the halftime trigger (`_roundsPlayed >= _mr`), and OT parity math. **Fix:** add a `_roundDecided` bool set on the first Live death, checked at the top of `OnPlayerDeath`, cleared in `OnRoundStart`.

**Other High items** (from per-area review, confirmed by synthesizer re-read): port double-free/reuse race between reaper `Stop` and concurrent user `Stop`/`Create` (`internal/ports/allocator.go:63-67`, no ownership check); reaper `Stop` vs concurrent `Restart`/`Status` TOCTOU producing spurious not-found errors.

---

## By area (Medium/Low detail)

**Security.** Beyond C1/C2/H1/H5/H7: in-game `!map` passes free-form player text to `ExecuteCommand` via `changelevel`/`host_workshop_map` (`Commands.cs`→`MapPool.cs:56-61`), allowing console-command chaining (`;`) on that one container (Medium; needs live confirmation of tokenization). The Discord admin gate (`handlers.go:66-100,328-334`) is correctly implemented but is **not** an authoritative boundary because the backing API trusts every caller. RCON password is plaintext in SQLite and container env (Low); `RCONPass` is correctly `json:"-"` (`model.go:77`).

**Concurrency.** The shared-`*Instance` race (H2) is the one that will trip `-race`. Port double-free/reuse race (High). Reaper-vs-Restart/Status TOCTOU. `store` uses `SetMaxOpenConns(1)` which serializes reads (Low perf) but correctly prevents write/write races.

**Reliability.** Reaper destroys healthy-but-RCON-unreachable servers + volumes (H3). Failed-provision leak + no reconciler (H8). `provision` under `context.Background`, not drained on shutdown; seed `ContainerWait` unbounded under `seedMu` (H9). RCON `Exec` reads one packet, so a `status` output >4KB truncates → can feed the reaper a false-empty count (Medium). `Stop` ignores `ContainerStop` error and force-removes, defeating the graceful 30s window (Low). Reaper loses in-memory idle timers on restart (Medium).

**Correctness.** Duel1v1 double-count (H10). `GameType==0 && GameMode==0` sentinel collides with legitimate Casual → silently rewritten to Competitive (Medium). `createRequest` numeric fields (`GameType`/`GameMode`/`MaxPlayers`/`BotQuota`) never range-validated (Medium). MapPool workshop id `3071005299` labeled `aim_redline` but resolves to `de_assembly`; other 3 ids unverified (Medium). Map-vote state is stale/cross-phase and `!map` bypasses the vote (Low). `EnsureTeam` dereferences `p.Controller` without the null guard used in `ZeroMoney` (Medium).

**Docker/Infra.** `docker/cs2/Dockerfile` base pinned to `:latest` (Medium, non-reproducible) and apt unpinned (Low); deploy Dockerfiles clean + pinned. `compose.yaml:6` and `seed.sh:6-7` still reference Metamod+CounterStrikeSharp though the stack moved to SwiftlyS2 (doc drift, Low). `fuse-overlayfs` launched without `-f`, then `chown`/`install_hooks` race the mount readiness (Medium). Headless workshop prewarm boots a server that can't reach Steam in the seed context, burning up to 4×300s (Medium) — **confirmed in production testing this session**.

**Go quality.** golangci-lint is the only failing gate — 9 errcheck unchecked deferred `Close()`/`ContainerRemove` + 2 staticcheck (`handlers.go:96` QF1012, `handlers.go:332` SA1019 deprecated `PermissionManageServer`). Hand-rolled stderr logger in `DockerManager` bypasses slog (`docker.go:167-171`). Dead code: `store.CountByOwner`, `gamemode.Preset.Cfg` (set/tested but never injected into the container), `rcon.typeResponseValue`, `ports.New` reserved variadic. Duplicated `CreateRequest`/`InstanceView` DTOs across `api` and `apiclient` risk wire-contract drift. Magic-number timeouts duplicated as bare literals.

**Tests.** Coverage is **inverted relative to risk**: `internal/api` 0% (incl. destructive stop-all + empty-owner_id cap bypass), `internal/reaper` 0% (the logic that destroys servers), `internal/bot` authz 0% (`isAdmin`/`guardOwnership`/`adminWide`), `DockerManager` lifecycle 8.1%, `rcon` wire protocol 15.4% (only `ParseStatus`), `apiclient` 0%, no C# test project at all. `config` 48.8%. `store` is solid at 81% but lacks migration-idempotency and List-ordering tests, and nothing asserts `RCONPass` never serializes to clients.

---

## Cross-cutting themes

1. **One missing boundary dominates.** The unauthenticated, host-exposed, root+docker.sock API is the single root cause behind ~6 findings. Fix auth + stop publishing the port and most of the Critical/High security surface collapses at once.
2. **The async `provision()` path is a recurring hazard** — it generates the data race, the no-drain-on-shutdown leak, the port/volume leak on failure, the `seedMu`-under-unbounded-wait stall, and "stuck in starting" orphans. A manager-lifetime context + WaitGroup + failure-path cleanup addresses the cluster.
3. **State leaks with no reconciler.** Ports, volumes, and store rows leak on failure/crash and nothing self-heals (`ListManagedContainers` is dead code). The system trends toward port exhaustion and orphan accumulation.
4. **RCON is a fragile single source of truth.** Destructive reaper decisions, player counts, and idle accounting all key on one RCON read that can be unreachable (treated as Online/0), truncated (>4KB single packet), or time-boxed by a shared dial deadline.
5. **The C# match state machine hangs off one unguarded counter.** `_roundsPlayed` advancing by exactly one per round is an unstated invariant; the missing round-decided latch breaks scoring, halftime, and OT simultaneously.
6. **Test coverage is inverted relative to risk** — the most dangerous code is the least tested, so every fix above can regress unnoticed.

---

## Quick wins (high value, low effort)

1. **Stop publishing the API port** (delete `compose.yaml:60`) — bot uses `cs2net` already; removes most external attack surface in one line.
2. **Un-publish RCON** — remove the `tcpRCON` entry from `bindings` (`docker.go:229-233`), keep it in `exposed`.
3. **Fix the data race** — `snapshot := *inst; go m.provision(...); return &snapshot, nil` (`docker.go:145-147`).
4. **Bound the request body + add server timeouts** — `http.MaxBytesReader` in `handleCreate`; `ReadTimeout`/`WriteTimeout`/`IdleTimeout` in `main.go:70-74`.
5. **Add a `_roundDecided` latch** in `Match.cs` — stops the double-count; high player-facing value.
6. **Reaper `OccupancyKnown`** — set true only after a successful `ParseStatus`; skip reaping on unknown occupancy. Prevents destroying live servers.
7. **Reject empty `owner_id` when `maxPer>0`** (`api.go:95`) — closes the trivial quota bypass independent of the larger auth work.
8. **Cleanup on failed provision** — `releasePorts(inst)` + `VolumeRemove(instVolume)` in the `StatusError` branch (`docker.go:154-158`).
9. **Clear the 11 golangci-lint issues** — `_ = x.Close()` on the 9 deferrals, `fmt.Fprintf(&sb, ...)` at `handlers.go:96`, `PermissionManageServer → PermissionManageGuild` at `handlers.go:332` — get CI to a clean gate.
10. **Fix the MapPool id/label** — `3071005299` resolves to `de_assembly`, not `aim_redline`; verify the other three ids against the live Workshop.
11. **Update stale docs** — `compose.yaml:6` and `seed.sh:6-7` still say Metamod+CounterStrikeSharp; the stack is SwiftlyS2.
12. **Null-guard `EnsureTeam`** — add `var c = p.Controller; if (c == null) return;` mirroring `ZeroMoney` (`Match.cs:139-143`).

---

## Suggested remediation order

1. **Lock the boundary (C1/C2/H1/H5/H6):** API auth token + unpublish API/RCON host ports + body limit/timeouts + reject empty `owner_id`. This neutralizes the unauthenticated-host-root chain.
2. **Stop destroying data (H3):** reaper `OccupancyKnown` + startup grace.
3. **Fix the async path (H2/H8/H9):** snapshot before goroutine, failure-path cleanup, lifetime context + WaitGroup drain, startup reconcile.
4. **Fix gameplay (H10 + Medium C# items):** round-decided latch, `EnsureTeam` null guard, MapPool ids.
5. **Harden containers (H7):** drop `SYS_ADMIN`/unconfined where possible; `no-new-privileges` + tailored profiles.
6. **Pay down quality/tests:** clear lint to a clean CI gate; add tests for the now-fixed api/reaper/lifecycle/authz paths and a `-race` regression test for H2; add a Duel1v1 unit-test project for the match state machine.

---

## Coverage gaps / residual risk

- **Automated adversarial verification did not run** (all 21 verifier agents failed to emit structured output); the synthesizer independently re-read and confirmed the Critical/High findings, but there was no separate refutation pass. Medium/Low findings carry single-reviewer confidence.
- **Not independently re-verified:** the `!map` `ExecuteCommand` console-tokenization (`;` chaining needs a live SwiftlyS2 build), exact CS2 designer names in `Weapons.cs`, the fuse-overlayfs mount-readiness race, the SwiftlyS2 `hotReload`/`Unload()` double-registration contract.
- **Not reviewed at all:** runtime/integration behavior, the `joedwards32/cs2:latest` base-image contents + `entry.sh` hook contract, SQLite file permissions on the `cs2-state` volume, multi-instance scale behavior of the sequential reaper tick, and validation of the three remaining workshop ids.

---

## What the codebase does well

- All hard gates green: `go build`/`vet`/`gofmt`/`test -race` clean, both plugins build 0/0.
- **Secret handling is clean:** gitleaks 0 leaks, `.env` gitignored, RCON passwords `crypto/rand`-generated per instance and excluded from API responses via `json:"-"`.
- Good architectural instincts: persist-before-start (recoverable mid-create crash), `Stop` treats already-removed containers as success, allocator reserves ports from the store on startup, SQLite uses WAL + `busy_timeout` + `SetMaxOpenConns(1)` (correctly prevents write races), API masks internal errors to clients while logging full detail.
- The Discord admin gate itself (`isAdmin` bitmask, `guardOwnership`, `OwnerScoped`) is correctly implemented — it just isn't the authoritative boundary.
- `store` is well-tested (81%); deploy Dockerfiles are pinned (`golang:1.26` + distroless) and lint-clean; structured `slog` logging is used nearly everywhere.
- The Duel1v1 OT/halftime math is self-consistent and well-commented once the counter bug (H10) is fixed.
