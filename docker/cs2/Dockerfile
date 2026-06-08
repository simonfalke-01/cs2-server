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

FROM joedwards32/cs2:latest

USER root

ENV HOMEDIR="/home/steam"

# .NET (bundled in the CSS "with-runtime" zip) needs ICU or invariant mode.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip libicu-dev \
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

# Install our pre/post hooks and the wrapper entrypoint.
COPY pre.sh post.sh mods-entrypoint.sh /opt/cs2-hooks/
RUN chmod +x /opt/cs2-hooks/*.sh && chown -R steam:steam /opt/cs2-hooks

# Directory where the orchestrator mounts user-supplied compiled plugins.
RUN mkdir -p /plugins && chown steam:steam /plugins

# Let .NET run without ICU; CSS only needs invariant globalization.
ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
ENV DOTNET_CLI_TELEMETRY_OPTOUT=1

USER steam
WORKDIR /home/steam

ENTRYPOINT ["/opt/cs2-hooks/mods-entrypoint.sh"]
