# CS2 dedicated server + Metamod:Source + CounterStrikeSharp
#
# Builds on the community CS2 dedicated server image (joedwards32/cs2), which
# downloads the ~60GB game via SteamCMD on first start into the volume mounted
# at /home/steam/cs2-dedicated/.
#
# The game's `csgo/` tree only exists at runtime (after SteamCMD), so we stage
# the mods inside the image at build time and copy them into place from a
# `pre.sh` hook that the base image sources after the download, just before the
# server starts.

# --- Stage 1: build the bundled sample CounterStrikeSharp plugin -----------
# Baked into the image so a fresh `docker compose up` ships a working plugin
# with no host-side build. Drop more plugins under plugins/ to bundle them too.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS plugins
WORKDIR /src
COPY plugins/ ./plugins/
RUN set -eux; \
    mkdir -p /out; \
    for proj in plugins/*/*.csproj; do \
        name="$(basename "$(dirname "$proj")")"; \
        dotnet build -c Release "$proj" -o "/build/$name"; \
        mkdir -p "/out/$name"; \
        cp "/build/$name/$name.dll" "/out/$name/"; \
    done

# --- Stage 2: the CS2 dedicated server image -------------------------------
FROM joedwards32/cs2:latest

USER root

ENV HOMEDIR="/home/steam"

# .NET (bundled in the CSS "with-runtime" zip) needs ICU or invariant mode.
# fuse-overlayfs is used in shared-game-files mode on rootless/userns Docker,
# where kernel OverlayFS mounts are not permitted.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip libicu-dev fuse-overlayfs \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Versions are pinned but overridable at build time, e.g.:
#   docker build --build-arg CSS_VERSION=v1.0.369 \
#                --build-arg CSS_FILE=counterstrikesharp-with-runtime-linux-1.0.369.zip ...
ARG METAMOD_BRANCH="2.0"
ARG CSS_VERSION="v1.0.369"
ARG CSS_FILE="counterstrikesharp-with-runtime-linux-1.0.369.zip"

# Stage Metamod + CounterStrikeSharp under /opt/cs2-mods/addons so pre.sh can
# copy them into the runtime-downloaded csgo/ tree.
RUN set -eux; \
    mkdir -p /opt/cs2-mods /tmp/mods; \
    cd /tmp/mods; \
    MM_PKG="$(curl -fsSL "https://mms.alliedmods.net/mmsdrop/${METAMOD_BRANCH}/mmsource-latest-linux")"; \
    echo "Metamod package: ${MM_PKG}"; \
    curl -fsSL "https://mms.alliedmods.net/mmsdrop/${METAMOD_BRANCH}/${MM_PKG}" -o metamod.tar.gz; \
    tar -xzf metamod.tar.gz -C /opt/cs2-mods; \
    curl -fsSL "https://github.com/roflmuffin/CounterStrikeSharp/releases/download/${CSS_VERSION}/${CSS_FILE}" -o css.zip; \
    unzip -oq css.zip -d /opt/cs2-mods; \
    cd /; rm -rf /tmp/mods; \
    chown -R steam:steam /opt/cs2-mods

# Install our hooks, helpers and the wrapper entrypoint.
# (Build context is the repo root so the plugins/ stage above can see sources.)
COPY docker/cs2/pre.sh docker/cs2/post.sh docker/cs2/mods-entrypoint.sh \
     docker/cs2/install-mods.sh docker/cs2/seed.sh /opt/cs2-hooks/
RUN chmod +x /opt/cs2-hooks/*.sh && chown -R steam:steam /opt/cs2-hooks

# Bundled, pre-built plugins (synced into CSS on boot by pre.sh).
COPY --from=plugins /out /opt/cs2-plugins
RUN chown -R steam:steam /opt/cs2-plugins

# Optional directory for extra user-supplied plugins (mounted by orchestrator).
RUN mkdir -p /plugins && chown steam:steam /plugins

# Mountpoints used in shared-game-files mode (OverlayFS):
#   /shared    read-only seeded game copy volume (lower at /shared/cs2)
#   /instance  per-instance writable layer (upper/work)
RUN mkdir -p /shared /instance && chown steam:steam /shared /instance

# Let .NET run without ICU; CSS only needs invariant globalization.
ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
ENV DOTNET_CLI_TELEMETRY_OPTOUT=1

USER steam
WORKDIR /home/steam

ENTRYPOINT ["/opt/cs2-hooks/mods-entrypoint.sh"]
