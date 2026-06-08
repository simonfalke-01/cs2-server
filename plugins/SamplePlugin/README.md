# SamplePlugin

A minimal [CounterStrikeSharp](https://docs.cssharp.dev/) plugin that proves
custom C# game logic loads inside the dedicated server.

It logs a banner on load, registers a `css_hello` command (`!hello` in chat),
and hooks `EventRoundStart`.

## Build

```bash
dotnet build -c Release
```

Output: `bin/Release/net10.0/SamplePlugin.dll`.

## Deploy (local iteration)

CounterStrikeSharp loads plugins from
`game/csgo/addons/counterstrikesharp/plugins/<PluginName>/<PluginName>.dll`.

The docker image copies any folder you place under the mounted `/plugins`
directory into that location on container start. So a layout like:

```
/plugins/
  SamplePlugin/
    SamplePlugin.dll
```

will be picked up automatically. The Makefile target `make plugins` builds this
plugin and stages it under `./plugins-dist/` ready to mount.

## Versioning

`CounterStrikeSharp.API` (NuGet) and the CSS runtime installed in the docker
image must match. Both are pinned to `1.0.369` (.NET 10). When bumping, update:

- `SamplePlugin.csproj` → `PackageReference Version`
- `docker/cs2/Dockerfile` → `CSS_VERSION` / `CSS_FILE`
