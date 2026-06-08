# SamplePlugin

A minimal [SwiftlyS2](https://swiftlys2.net/) plugin that proves custom C# game
logic loads inside the dedicated server.

It logs a banner on load, registers a `sw_hello` command (`!hello` in chat), and
hooks `EventRoundStart`.

> We use SwiftlyS2 rather than CounterStrikeSharp because CSS is currently broken
> on recent CS2 builds (see https://github.com/ianlucas/cs2-signatures), while
> SwiftlyS2 loads and runs. SwiftlyS2 also doesn't require Metamod.

## Build

```bash
dotnet publish -c Release -o out
```

Output: a folder with `SamplePlugin.dll` + its deps.

## Deploy (local iteration)

SwiftlyS2 loads plugins from
`game/csgo/addons/swiftlys2/plugins/<PluginName>/`.

The docker image bundles this plugin automatically (built in a stage and copied
to `/opt/cs2-plugins`, then synced on boot). You can also drop extra published
plugin folders under the mounted `/plugins` directory and they'll be synced too.

## Versioning

`SwiftlyS2.CS2` (NuGet) must match the SwiftlyS2 build installed in the docker
image. Both are pinned to `1.3.5`. To bump:

- `SamplePlugin.csproj` → `PackageReference Version`
- `docker/cs2/Dockerfile` → `SWIFTLY_VERSION` / `SWIFTLY_FILE`
