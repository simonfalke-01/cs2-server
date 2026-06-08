#!/bin/bash
# install-mods.sh — install/refresh Metamod:Source + CounterStrikeSharp into a
# given csgo/ directory and patch gameinfo.gi (idempotent).
#
# Usage: install-mods.sh <path-to-game/csgo>
#
# Used by:
#   - seed.sh  (bakes mods into the shared read-only game copy)
#   - pre.sh   (per-instance, non-shared mode)
set -euo pipefail

CSGO_DIR="${1:?usage: install-mods.sh <csgo-dir>}"
ADDONS_SRC="/opt/cs2-mods/addons"
GAMEINFO="${CSGO_DIR}/gameinfo.gi"

if [[ ! -d "${CSGO_DIR}" ]]; then
    echo "[mods] ERROR: ${CSGO_DIR} not found; game files missing?" >&2
    return 1 2>/dev/null || exit 1
fi

mkdir -p "${CSGO_DIR}/addons"

if [[ -d "${ADDONS_SRC}" ]]; then
    # -a preserves perms; refreshes the framework so image version bumps apply.
    cp -a "${ADDONS_SRC}/." "${CSGO_DIR}/addons/"
    echo "[mods] addons/ synced from image."
else
    echo "[mods] WARNING: staged mods not found at ${ADDONS_SRC}"
fi

# --- Patch gameinfo.gi to load Metamod (idempotent) -----------------------
METAMOD_LINE='			Game	csgo/addons/metamod'
if [[ -f "${GAMEINFO}" ]]; then
    if grep -q 'csgo/addons/metamod' "${GAMEINFO}"; then
        echo "[mods] gameinfo.gi already patched."
    else
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
            echo "[mods] WARNING: failed to patch gameinfo.gi automatically." >&2
        fi
    fi
else
    echo "[mods] WARNING: ${GAMEINFO} not found; cannot patch for Metamod." >&2
fi

echo "[mods] Mod installation complete."
