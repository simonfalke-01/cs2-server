#!/bin/bash
# pre.sh — sourced by the base image's entry.sh AFTER SteamCMD has run and the
# working directory is ${STEAMAPPDIR}/game/.
#
# Modes:
#   - Non-shared (default): install/refresh Metamod + CounterStrikeSharp into
#     this instance's own game copy, patch gameinfo.gi, sync plugins.
#   - Shared (CS2_SHARED_MODE=1): mods are already baked into the shared,
#     read-only lower layer, so we only sync per-instance plugins.
#
# This script is `source`d, so avoid `exit`; use `return` on fatal errors.

CSGO_DIR="${STEAMAPPDIR}/game/csgo"

if [[ ! -d "${CSGO_DIR}" ]]; then
    echo "[mods] ERROR: ${CSGO_DIR} not found; game files missing?"
    return 0
fi

if [[ "${CS2_SHARED_MODE:-0}" != "1" ]]; then
    # Per-instance install (each server owns a full game copy).
    /opt/cs2-hooks/install-mods.sh "${CSGO_DIR}" || echo "[mods] install-mods failed"
    # Marker used by the orchestrator's seeding step to know the shared game
    # copy has game files + mods + a patched gameinfo.gi ready to share.
    touch "${STEAMAPPDIR}/.cs2-seeded" 2>/dev/null || true
else
    echo "[mods] Shared mode: mods provided by read-only base layer."
fi

# --- Sync plugins (both modes) -------------------------------------------
# Each plugin is a folder containing <Name>.dll. We sync, in order:
#   1. plugins baked into the image at /opt/cs2-plugins (the bundled sample)
#   2. extra user plugins optionally mounted at /plugins by the orchestrator
CSS_PLUGINS_DIR="${CSGO_DIR}/addons/counterstrikesharp/plugins"
mkdir -p "${CSS_PLUGINS_DIR}"
if [[ -d /opt/cs2-plugins ]] && [[ -n "$(ls -A /opt/cs2-plugins 2>/dev/null)" ]]; then
    cp -a /opt/cs2-plugins/. "${CSS_PLUGINS_DIR}/"
    echo "[mods] Bundled plugins synced from /opt/cs2-plugins."
fi
if [[ -d /plugins ]] && [[ -n "$(ls -A /plugins 2>/dev/null)" ]]; then
    cp -a /plugins/. "${CSS_PLUGINS_DIR}/"
    echo "[mods] User plugins synced from /plugins."
fi

echo "[mods] pre.sh complete."
