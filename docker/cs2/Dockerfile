# CS2 dedicated server + SwiftlyS2 (C# scripting framework)
#
# Builds on the community CS2 dedicated server image (joedwards32/cs2), which
# downloads the ~60GB game via SteamCMD on first start into the volume mounted
# at /home/steam/cs2-dedicated/.
#
# We use SwiftlyS2 instead of Metamod+CounterStrikeSharp: CSS is currently broken
# on recent CS2 builds (see github.com/ianlucas/cs2-signatures), while SwiftlyS2
# is green and loads directly via gameinfo.gi (no Metamod needed).
#
# The game's `csgo/` tree only exists at runtime (after SteamCMD), so we stage
# the framework + plugins inside the image and copy them into place from a
# `pre.sh` hook the base image sources just before the server starts.

# --- Stage 1: build the bundled sample SwiftlyS2 plugin --------------------
# Baked into the image so a fresh `docker compose up` ships a working plugin
# with no host-side build. Add more plugins under plugins/ to bundle them too.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS plugins
WORKDIR /src
COPY plugins/ ./plugins/
RUN set -eux; \
    mkdir -p /out; \
    for proj in plugins/*/*.csproj; do \
        name="$(basename "$(dirname "$proj")")"; \
        dotnet publish -c Release "$proj" -o "/out/$name"; \
    done

# --- Stage 2: the CS2 dedicated server image -------------------------------
FROM joedwards32/cs2:latest

USER root

ENV HOMEDIR="/home/steam"

# fuse-overlayfs is used in shared-game-files mode on rootless/userns Docker,
# where kernel OverlayFS mounts are not permitted. SwiftlyS2 ships its own .NET
# runtime (the "-with-runtimes" release), so no system .NET is required.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip libicu-dev fuse-overlayfs \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# SwiftlyS2 version (overridable at build time).
ARG SWIFTLY_VERSION="v1.3.5"
ARG SWIFTLY_FILE="swiftlys2-linux-v1.3.5-with-runtimes.zip"

# Stage SwiftlyS2 under /opt/cs2-mods/addons so pre.sh can copy it into the
# runtime-downloaded csgo/ tree.
RUN set -eux; \
    mkdir -p /opt/cs2-mods /tmp/mods; \
    cd /tmp/mods; \
    curl -fsSL --retry 5 --retry-all-errors --retry-delay 3 \
        "https://github.com/swiftly-solution/swiftlys2/releases/download/${SWIFTLY_VERSION}/${SWIFTLY_FILE}" -o sw.zip; \
    unzip -oq sw.zip; \
    cp -a swiftlys2-*/addons /opt/cs2-mods/; \
    cd /; rm -rf /tmp/mods; \
    chown -R steam:steam /opt/cs2-mods

# Install our hooks, helpers and the wrapper entrypoint.
# (Build context is the repo root so the plugins/ stage above can see sources.)
COPY docker/cs2/pre.sh docker/cs2/post.sh docker/cs2/mods-entrypoint.sh \
     docker/cs2/install-mods.sh docker/cs2/seed.sh docker/cs2/prewarm-workshop.sh /opt/cs2-hooks/
RUN chmod +x /opt/cs2-hooks/*.sh && chown -R steam:steam /opt/cs2-hooks

# Bundled, pre-built plugins (synced into SwiftlyS2's plugins dir on boot).
COPY --from=plugins /out /opt/cs2-plugins
RUN chown -R steam:steam /opt/cs2-plugins

# Game-mode cfg bundles (synced into csgo/cfg/ on boot; pre.sh wires the active
# mode's cfg into server.cfg). Works in shared mode too (synced per-instance).
COPY docker/cs2/cfg /opt/cs2-cfg
RUN chown -R steam:steam /opt/cs2-cfg

# Optional directory for extra user-supplied plugins (mounted by orchestrator).
RUN mkdir -p /plugins && chown steam:steam /plugins

# Mountpoints used in shared-game-files mode (OverlayFS):
#   /shared    read-only seeded game copy volume (lower at /shared/cs2)
#   /instance  per-instance writable layer (upper/work)
RUN mkdir -p /shared /instance && chown steam:steam /shared /instance

ENV DOTNET_CLI_TELEMETRY_OPTOUT=1

USER steam
WORKDIR /home/steam

ENTRYPOINT ["/opt/cs2-hooks/mods-entrypoint.sh"]
