# Duel1v1

A strict **two-player 1v1 duel** game mode for CS2, built on
[SwiftlyS2](https://swiftlys2.net/). Configured entirely from in-server chat.

Gated by the `CS2_MODE` environment variable (set by the orchestrator from the
requested game-mode preset): it only activates when `CS2_MODE=1v1`, so the same
game image can serve every mode — on any other mode the plugin loads but stays
dormant.

## Flow

1. **Warmup** (default) — infinite, free practice with instant respawn. The first
   two humans to connect are the players (P1 starts CT, P2 starts T); anyone else
   spectates. Configure the match, then **both players type `!ready`** to start.
2. **Live** — MR match. A round ends when a player dies; the survivor scores.
   - `MRn` ⇒ first to `n+1` wins, halftime side-swap after `n` rounds.
   - Default `MR12` ⇒ first to 13, swap at 12.
   - **No round timer** (rounds end only on a kill).
   - **Sides swap once at halftime only** — not every round.
3. **Overtime** — on a regulation tie (`n-n`), MR3 overtime (first to 4, swap at
   3), repeating until someone wins.
4. **Match end** — winner announced; both players `!rematch` to replay with the
   same settings, otherwise it returns to warmup.

A player disconnecting mid-match **aborts to warmup** and resets the score; the
next-longest-connected spectator is promoted into the open slot.

## Loadout & buying

- Buying is **disabled** (the cfg gives `$0`/no buytime). Pressing **B** shows a
  hint pointing at the weapon commands.
- Each player picks weapons via chat; picks **persist** across rounds and apply
  **immediately** (even mid-round). Defaults: **AK-47** primary, **Desert Eagle**
  secondary, plus a knife.
- **Armor** and **grenades** are server-wide toggles, changeable in warmup only.
  When grenades are on, both players get the standard kit each round.

## Commands

All work as `!cmd` and `/cmd` in chat (and as console commands).

| Command | When | Who | Effect |
|---|---|---|---|
| `!ready` / `!unready` | warmup | players | ready up / cancel; both ready ⇒ start |
| `!mr <1-30>` | warmup | either | set MR (default 12) |
| `!nades` | warmup | either | toggle grenades (server-wide) |
| `!armor` | warmup | either | toggle armor (server-wide, default on) |
| `!map <name>` | warmup | either | change map (pool or any name) |
| `!maps` | any | any | list the curated 1v1 map pool |
| `!votemap <name>` | warmup | players | both vote ⇒ change map |
| `!ak !m4 !awp !deagle …` | anytime | per-player | set your primary/secondary (immediate) |
| `!guns` | any | any | list all weapon commands |
| `!score` | any | any | current score + phase |
| `!rematch` | match end | players | both ⇒ replay same settings |
| `!help` | any | any | command summary |

See `Weapons.cs` for the full weapon alias list (full CS2 arsenal) and
`MapPool.cs` for the curated map pool (stock + workshop maps).

## Build

```bash
dotnet publish -c Release -o out
```

The docker image bundles this plugin automatically (built in the `plugins/*`
stage of `docker/cs2/Dockerfile` and synced into SwiftlyS2's plugins dir on
boot). The supporting round-flow convars are in `docker/cs2/cfg/1v1.cfg`.

## Versioning

`SwiftlyS2.CS2` (NuGet) must match the SwiftlyS2 build in the docker image; both
pinned to `1.3.5`. To bump, update `Duel1v1.csproj` and `docker/cs2/Dockerfile`
(`SWIFTLY_VERSION` / `SWIFTLY_FILE`) together.
