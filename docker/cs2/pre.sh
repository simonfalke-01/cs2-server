#!/bin/bash
# pre.sh — sourced by the base image's entry.sh AFTER SteamCMD has downloaded
# the game and the working directory is ${STEAMAPPDIR}/game/.
#
# Responsibilities:
#   1. Install/refresh Metamod:Source + CounterStrikeSharp into csgo/addons.
#   2. Patch csgo/gameinfo.gi so the engine loads Metamod (idempotent).
#   3. Sync user-supplied compiled plugins from /plugins into CSS plugins dir.
#
# This script is `source`d, so avoid `exit`; use `return` on fatal errors.

CSGO_DIR="${STEAMAPPDIR}/game/csgo"
ADDONS_SRC="/opt/cs2-mods/addons"
GAMEINFO="${CSGO_DIR}/gameinfo.gi"

echo "[mods] Installing Metamod:Source + CounterStrikeSharp..."

if [[ ! -d "${CSGO_DIR}" ]]; then
    echo "[mods] ERROR: ${CSGO_DIR} not found; game files missing?"
    return 0
fi

mkdir -p "${CSGO_DIR}/addons"

if [[ -d "${ADDONS_SRC}" ]]; then
    # -a preserves perms; this refreshes the framework on every boot so version
    # bumps in the image take effect without wiping user data.
    cp -a "${ADDONS_SRC}/." "${CSGO_DIR}/addons/"
    echo "[mods] addons/ synced from image."
else
    echo "[mods] WARNING: staged mods not found at ${ADDONS_SRC}"
fi

# --- Patch gameinfo.gi to load Metamod (idempotent) -----------------------
# The engine reads SearchPaths from gameinfo.gi. We insert a line pointing at
# csgo/addons/metamod just above the default "Game  csgo" entry.
METAMOD_LINE='			Game	csgo/addons/metamod'

if [[ -f "${GAMEINFO}" ]]; then
    if grep -q 'csgo/addons/metamod' "${GAMEINFO}"; then
        echo "[mods] gameinfo.gi already patched."
    else
        # Insert our line immediately before the first '			Game	csgo' entry.
        # Use a tab-aware match; gameinfo.gi uses tab indentation.
        awk -v ins="${METAMOD_LINE}" '
            !done && $0 ~ /[[:space:]]Game[[:space:]]+csgo[[:space:]]*$/ {
                print ins
                done=1
            }
            { print }
        ' "${GAMEINFO}" > "${GAMEINFO}.tmp" && mv "${GAMEINFO}.tmp" "${GAMEINFO}"

        if grep -q 'csgo/addons/metamod' "${GAMEINFO}"; then
            echo "[mods] gameinfo.gi patched for Metamod."
        else
            echo "[mods] WARNING: failed to patch gameinfo.gi automatically."
        fi
    fi
else
    echo "[mods] WARNING: ${GAMEINFO} not found; cannot patch for Metamod."
fi

# --- Sync user plugins ----------------------------------------------------
# Compiled CounterStrikeSharp plugins are mounted read-only at /plugins by the
# orchestrator. Each plugin is a folder containing <Name>.dll.
CSS_PLUGINS_DIR="${CSGO_DIR}/addons/counterstrikesharp/plugins"
if [[ -d /plugins ]] && [[ -n "$(ls -A /plugins 2>/dev/null)" ]]; then
    mkdir -p "${CSS_PLUGINS_DIR}"
    cp -a /plugins/. "${CSS_PLUGINS_DIR}/"
    echo "[mods] User plugins synced from /plugins."
fi

echo "[mods] Mod installation complete."
