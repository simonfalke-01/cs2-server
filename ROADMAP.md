# Roadmap

Status of the cs2-server platform against the original goal: a **Go control
plane** (Discord bot + orchestrator) that spins up **on-demand CS2 servers**
running the **SwiftlyS2** framework for **custom C# gameplay**, public/private
configurable, Docker-first with a path to **Kubernetes + Agones**.

> Plugin layer note: we use **SwiftlyS2** instead of CounterStrikeSharp because
> CSS is currently broken on recent CS2 builds (see
> [ianlucas/cs2-signatures](https://github.com/ianlucas/cs2-signatures)).
> SwiftlyS2 loads via gameinfo.gi (no Metamod) and works on the current build.

This document tracks what's done and what's upcoming/unimplemented.

## Legend

- [x] Implemented
- [ ] Not yet implemented
- 🔶 Partially implemented (details inline)

---

## Implemented (MVP)

- [x] Modded CS2 docker image: SwiftlyS2 install, idempotent `gameinfo.gi`
  patch, bundled + mounted plugin sync (`docker/cs2/`)
- [x] Sample SwiftlyS2 C# plugin proving custom logic loads, verified end-to-end
  on the current CS2 build (`plugins/SamplePlugin`)
- [x] `ServerManager` interface + Docker backend (`internal/orchestrator`)
- [x] SQLite instance store, UDP/TCP port allocator
  (`internal/store`, `internal/ports`)
- [x] Source RCON client + `status` parser (`internal/rcon`)
- [x] Orchestrator HTTP API with health check (`internal/api`)
- [x] Discord bot: `/create /list /status /restart /stop` (`internal/bot`)
- [x] Idle-server auto-shutdown reaper (`internal/reaper`)
- [x] Configurable public (GSLT) vs private/LAN servers
- [x] **Shared game files (OverlayFS)** — opt-in (`CS2C_SHARED_GAME_FILES`): one
  seeded read-only game copy shared by all instances + thin per-instance writable
  layer; auto-seeds on first create; fast (seconds) server starts
- [x] Unit tests (ports, rcon, store); compose files + env examples

---

## Phase A — Hardening the control plane (next up)

High-value gaps in what already exists.

- [ ] **API authentication.** `internal/api` has no auth; any caller can
  create/stop servers. Add a shared-secret/bearer token (or mTLS) between bot
  and orchestrator. _Touches: `internal/api`, `internal/apiclient`, `config`._
- [ ] **Startup reconciliation.** `DockerManager.ListManagedContainers` exists
  but is unused. On boot, reconcile DB rows against live containers (adopt
  orphans, mark/cleanup dead ones, re-reserve ports). _Touches:
  `internal/orchestrator/docker.go`, `cmd/orchestrator`._
- [ ] **Accurate status lifecycle.** Instances are marked `running` immediately
  after container start; there's no `starting → running` transition tied to
  SteamCMD download / RCON readiness, and no `error`/exited detection from
  container events. Add a watcher on Docker events. _Touches:
  `internal/orchestrator`, `internal/store`._
- [ ] **Per-request GSLT pass-through.** 🔶 The API accepts `gslt` and the
  manager falls back to `CS2C_DEFAULT_GSLT`, but the bot's `/create` never sends
  one (`req.GSLT` is always empty — see `internal/bot/handlers.go`). Add a
  secure way to supply per-server GSLTs (bot option is insecure in chat; prefer
  a registered pool/secret store). _Touches: `internal/bot`, `config`._
- [ ] **Graceful create rollback / readiness wait.** `Create` returns before the
  server is actually joinable. Optionally poll RCON until ready and report a
  real connect-ready signal.
- [ ] **Structured request logging + correlation IDs** across bot → API → Docker.

## Phase B — Operability

- [ ] **Metrics** (`/metrics` Prometheus): instance counts, port-pool usage,
  create/stop latency, reaper actions.
- [ ] **Log streaming / retrieval.** Fetch recent container logs via API and a
  `/logs` Discord command.
- [ ] **Crash auto-recovery policy** beyond Docker's `unless-stopped`
  (backoff, max-retries, alerting to a Discord channel).
- [ ] **CI/CD**: GitHub Actions to `go test`/`vet`, build & push the game image
  and control-plane images, and build the sample plugin.
- [ ] **Config validation & dry-run** on orchestrator startup (verify image
  exists, data paths writable, port range sane).

## Phase C — Gameplay & server features

- 🔶 **Steam Workshop maps/collections.** Single Workshop map is wired:
  `/create workshop:<id>` (API `workshop_map`) sets `CS2_HOST_WORKSHOP_MAP`, and
  the 1v1 plugin changes maps in-game via `ds_workshop_changelevel`. _Still TODO:
  collection (`CS2_HOST_WORKSHOP_COLLECTION`) support + a general `/map` command
  for non-1v1 servers._
- [ ] **SourceTV / demo recording.** Wire `TV_ENABLE/TV_PORT/TV_AUTORECORD`
  through the orchestrator and allocate the extra UDP port; optional demo upload
  in `post.sh`.
- [x] **Game-mode presets** (competitive / wingman / deathmatch / **1v1**) as a
  single `/create mode:` choice mapping to `game_type`/`game_mode`/cfg bundles.
  Presets live in `internal/gamemode` (shared by api/bot/orchestrator); cfg
  bundles in `docker/cs2/cfg/`; default via `CS2C_DEFAULT_MODE`. The `1v1` mode
  ships a two-player duel SwiftlyS2 plugin (`plugins/Duel1v1`, gated on
  `CS2_MODE`): warmup/!ready, MR scoring + halftime swap + overtime, per-player
  chat weapon picks, buying disabled, in-game map controls. _Extend with
  retake/surf presets as needed._
- [ ] **Plugin management.** Per-server plugin selection, upload/registry of
  SwiftlyS2 plugins, and hot-reload, instead of the single bundled set.
- [ ] **Live config tuning** over RCON (change map, kick, exec cfg) from Discord.
- [ ] **`SteamAPIKey` usage.** Loaded in config but unused — wire up for
  player/stat lookups or workshop API.

## Phase D — Multi-user & quotas

- [ ] **Richer ownership/permissions.** Bot is owner-scoped, but there are no
  Discord role gates (admin vs user), shared/team servers, or audit log.
- [ ] **Quotas & lifetimes**: max-lifetime TTL, scheduled servers, cost/usage
  accounting per user.
- [ ] **Reservation/queue** when the port pool or host capacity is exhausted.

## Phase E — Scale-out: Kubernetes + Agones

The `ServerManager` interface is the seam (see
`internal/orchestrator/agones_stub.go`).

- [ ] **Agones backend** implementing `ServerManager` (Fleet +
  GameServerAllocation, status→Instance mapping, SDK health/player count).
- [ ] **Backend selection** via config (`CS2C_BACKEND=docker|agones`).
- [ ] **Multi-host scheduling** and per-instance UDP port allocation via Agones.
- [ ] **Helm chart / manifests** for the control plane + game-server Fleet.
- [ ] **Autoscaling** (Fleet autoscaler) and node pool sizing guidance.

## Phase F — Distribution & docs

- [ ] **Published images** (GHCR) for game + control plane, versioned releases.
- [ ] **Architecture & operator docs** (runbook, scaling, security hardening).
- [ ] **End-to-end integration test** that boots the image and asserts SwiftlyS2
  + `SamplePlugin` load (currently a manual smoke test in the README).

---

## Known limitations / tech debt

- No authentication on the orchestrator API (Phase A).
- Disk: in the default (non-shared) mode each server stores its own ~40–60GB
  game copy. Enable `CS2C_SHARED_GAME_FILES=true` to share one copy (needs a
  Linux host with OverlayFS or fuse-overlayfs).
- `Status` returns `online` even when RCON isn't ready yet (best-effort).
- Plugins are the same bundled set for all servers (Phase C).
- Framework/build coupling: a CS2 update can break the plugin framework's
  signatures until upstream updates (see the signatures tracker). `SwiftlyS2.CS2`
  NuGet and the framework baked into the image must match (both pinned `1.3.5`).
