# Arena1v1

A winner-stays **1v1 arena** game mode for CS2, built on
[SwiftlyS2](https://swiftlys2.net/). It pairs two players into a duel each round;
the survivor stays as "champion" and the next player in the queue rotates in.

This plugin is **gated by the `CS2_MODE` environment variable** (set by the
orchestrator from the requested game-mode preset). It only activates when
`CS2_MODE=1v1`, so the same game image can serve every mode — on any other mode
the plugin loads but stays dormant.

## Behavior

- Tracks connected human players in a FIFO queue (bots are ignored).
- On each `round_start`, two players are assigned to the arena (T vs CT) and the
  rest are benched in spectator.
- On `player_death`, the surviving combatant becomes the champion and the dead
  player goes to the back of the queue.
- The champion stays next round; the second slot is filled from the queue
  (classic winner-stays rotation).
- Players that disconnect are pruned from the champion/combatant/queue state.

The supporting round ruleset (no buying, short freeze time, no weapon drops,
friendly-fire off, etc.) is applied by the bundled `1v1.cfg`
(`docker/cs2/cfg/1v1.cfg`), execed by the server via `server.cfg`. This plugin
owns only the orchestration.

## Commands

- `!arena` / `sw_arena` — show the current matchup and queue sizes.
- `!queue` / `sw_queue` — show how many players are waiting.

## Build

```bash
dotnet publish -c Release -o out
```

The docker image bundles this plugin automatically (built in the `plugins/*`
stage of `docker/cs2/Dockerfile` and synced into SwiftlyS2's plugins dir on
boot), alongside `SamplePlugin`.

## Versioning

`SwiftlyS2.CS2` (NuGet) must match the SwiftlyS2 build installed in the docker
image. Both are pinned to `1.3.5`. To bump, update `Arena1v1.csproj` and
`docker/cs2/Dockerfile` (`SWIFTLY_VERSION` / `SWIFTLY_FILE`) together.

## Notes / tuning

The pairing/queue logic is map-agnostic: it uses the map's normal spawn points
via `SwitchTeam`, so two combatants spawn at standard T/CT spawns. For dedicated
arena spawn positions you can extend `SetupArena` to `Teleport` players to
per-map points (the `IPlayer.Teleport(Vector?, QAngle?, Vector?)` API is
available). This was kept simple intentionally; final tuning happens on a live
server.
