#!/bin/bash
# seed.sh — build the SHARED, read-only game copy used by all instances in
# shared-game-files mode. Runs once (the orchestrator invokes a throwaway
# container with this as its command).
#
# It downloads CS2 via SteamCMD into <target>, installs Metamod +
# CounterStrikeSharp, patches gameinfo.gi, and writes a .cs2-seeded marker.
#
# Usage: seed.sh [target-dir]   (default: /shared/cs2)
set -euo pipefail

TARGET="${1:-/shared/cs2}"
STEAMCMDDIR="${STEAMCMDDIR:-/home/steam/steamcmd}"
STEAMAPPID="${STEAMAPPID:-730}"

echo "[seed] Seeding shared CS2 install into ${TARGET}"
mkdir -p "${TARGET}"

# Download / update the game (no forced validate; this is the slow ~40GB step).
MAX_ATTEMPTS=3
attempt=0
rc=1
while [[ $rc != 0 ]] && [[ $attempt -lt $MAX_ATTEMPTS ]]; do
    ((attempt+=1))
    [[ $attempt -gt 1 ]] && echo "[seed] Retrying SteamCMD (attempt ${attempt})"
    bash "${STEAMCMDDIR}/steamcmd.sh" \
        +force_install_dir "${TARGET}" \
        +@bClientTryRequestManifestWithoutCode 1 \
        +login anonymous \
        +app_update "${STEAMAPPID}" \
        +quit && rc=0 || rc=$?
done
if [[ $rc != 0 ]]; then
    echo "[seed] ERROR: SteamCMD failed after ${MAX_ATTEMPTS} attempts" >&2
    exit $rc
fi

# steamclient.so fix (mirrors the base image entry.sh).
mkdir -p "${HOME:-/home/steam}/.steam/sdk64"
ln -sfT "${STEAMCMDDIR}/linux64/steamclient.so" "${HOME:-/home/steam}/.steam/sdk64/steamclient.so"

# Pre-cache the curated Steam Workshop pool into the shared copy so every
# instance overlays maps already on disk (instant !map switches). Boots a
# vanilla server per id BEFORE mods are installed; best-effort (never fatal).
# shellcheck source=/dev/null
source /opt/cs2-hooks/prewarm-workshop.sh
prewarm_workshop "${TARGET}/game"

# Bake mods + gameinfo patch into the shared copy.
/opt/cs2-hooks/install-mods.sh "${TARGET}/game/csgo"

touch "${TARGET}/.cs2-seeded"
echo "[seed] Shared install ready at ${TARGET}"
