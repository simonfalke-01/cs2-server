#!/bin/bash
# install-mods.sh — install/refresh the SwiftlyS2 framework into a given csgo/
# directory and patch gameinfo.gi (idempotent).
#
# Why SwiftlyS2 (not Metamod + CounterStrikeSharp)?
#   CounterStrikeSharp is currently broken against recent CS2 builds (see the
#   signature tracker at github.com/ianlucas/cs2-signatures — CSS shows a
#   warning/red status, while SwiftlyS2 is green). SwiftlyS2 is a Source2
#   scripting framework with first-class C# plugin support that loads directly
#   via gameinfo.gi (it does NOT need Metamod), which also sidesteps Metamod's
#   plugin-folder resolution issues on CS2 dedicated servers.
#
# Usage: install-mods.sh <path-to-game/csgo>
#
# Used by:
#   - seed.sh  (bakes the framework into the shared read-only game copy)
#   - pre.sh   (per-instance, non-shared mode)
set -euo pipefail

CSGO_DIR="${1:?usage: install-mods.sh <csgo-dir>}"
ADDONS_SRC="/opt/cs2-mods/addons"
GAMEINFO="${CSGO_DIR}/gameinfo.gi"

if [[ ! -d "${CSGO_DIR}" ]]; then
    echo "[mods] ERROR: ${CSGO_DIR} not found; game files missing?" >&2
    # Works whether this script is sourced (return) or executed (exit). The
    # `exit 1` only looks unreachable to shellcheck because `return` outside a
    # function/sourced context is a runtime decision.
    # shellcheck disable=SC2317
    return 1 2>/dev/null || exit 1
fi

mkdir -p "${CSGO_DIR}/addons"

if [[ -d "${ADDONS_SRC}" ]]; then
    # -a preserves perms; refreshes the framework so image version bumps apply.
    cp -a "${ADDONS_SRC}/." "${CSGO_DIR}/addons/"
    echo "[mods] addons/ (SwiftlyS2) synced from image."
else
    echo "[mods] WARNING: staged mods not found at ${ADDONS_SRC}"
fi

# --- Patch gameinfo.gi to load SwiftlyS2 (idempotent) ---------------------
# Per the SwiftlyS2 docs, add "Game csgo/addons/swiftlys2" immediately BELOW the
# "Game_LowViolence csgo_lv" line in the SearchPaths block.
SWIFTLY_LINE='			Game	csgo/addons/swiftlys2'
if [[ -f "${GAMEINFO}" ]]; then
    # Drop any pre-existing swiftlys2 (and legacy metamod) search-path lines,
    # then insert swiftlys2 right after Game_LowViolence. Idempotent + repairs
    # a previously mis-placed entry.
    awk -v ins="${SWIFTLY_LINE}" '
        $0 ~ /Game[[:space:]]+csgo\/addons\/swiftlys2[[:space:]]*$/ { next }
        $0 ~ /Game[[:space:]]+csgo\/addons\/metamod[[:space:]]*$/ { next }
        { print }
        $0 ~ /Game_LowViolence[[:space:]]+csgo_lv/ { print ins }
    ' "${GAMEINFO}" > "${GAMEINFO}.tmp" && mv "${GAMEINFO}.tmp" "${GAMEINFO}"

    if grep -q 'csgo/addons/swiftlys2' "${GAMEINFO}"; then
        echo "[mods] gameinfo.gi patched for SwiftlyS2."
    else
        echo "[mods] WARNING: failed to patch gameinfo.gi automatically." >&2
    fi
else
    echo "[mods] WARNING: ${GAMEINFO} not found; cannot patch for SwiftlyS2." >&2
fi

echo "[mods] Mod installation complete."
