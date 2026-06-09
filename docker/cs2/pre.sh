#!/bin/bash
# pre.sh — sourced by the base image's entry.sh AFTER SteamCMD has run and the
# working directory is ${STEAMAPPDIR}/game/.
#
# Modes:
#   - Non-shared (default): install/refresh SwiftlyS2 into this instance's own
#     game copy, patch gameinfo.gi, sync plugins.
#   - Shared (CS2_SHARED_MODE=1): the framework is already baked into the shared,
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

    # Pre-cache the curated Steam Workshop pool once per instance (best-effort;
    # never fatal). Shared mode skips this — those maps are already baked into
    # the read-only lower layer by seed.sh.
    if [[ ! -f "${STEAMAPPDIR}/.ws-prewarmed" ]]; then
        # shellcheck source=/dev/null
        source /opt/cs2-hooks/prewarm-workshop.sh
        prewarm_workshop "${STEAMAPPDIR}/game"
        touch "${STEAMAPPDIR}/.ws-prewarmed" 2>/dev/null || true
    fi
else
    echo "[mods] Shared mode: mods provided by read-only base layer."
fi

# --- Sync plugins (both modes) -------------------------------------------
# Each plugin is a folder (the `dotnet publish` output of a SwiftlyS2 plugin).
# We sync, in order:
#   1. plugins baked into the image at /opt/cs2-plugins (the bundled sample)
#   2. extra user plugins optionally mounted at /plugins by the orchestrator
SW_PLUGINS_DIR="${CSGO_DIR}/addons/swiftlys2/plugins"
mkdir -p "${SW_PLUGINS_DIR}"
if [[ -d /opt/cs2-plugins ]] && [[ -n "$(ls -A /opt/cs2-plugins 2>/dev/null)" ]]; then
    cp -a /opt/cs2-plugins/. "${SW_PLUGINS_DIR}/"
    echo "[mods] Bundled plugins synced from /opt/cs2-plugins."
fi
if [[ -d /plugins ]] && [[ -n "$(ls -A /plugins 2>/dev/null)" ]]; then
    cp -a /plugins/. "${SW_PLUGINS_DIR}/"
    echo "[mods] User plugins synced from /plugins."
fi

# --- Game-mode cfg bundles (both modes) ----------------------------------
# Install the mode cfg bundles baked into the image at /opt/cs2-cfg into
# csgo/cfg/, then make server.cfg exec the requested mode's cfg. CS2 execs
# csgo/cfg/server.cfg on map load, so this applies the mode ruleset every map.
CFG_DIR="${CSGO_DIR}/cfg"
if [[ -d /opt/cs2-cfg ]] && [[ -n "$(ls -A /opt/cs2-cfg 2>/dev/null)" ]]; then
    mkdir -p "${CFG_DIR}"
    cp -a /opt/cs2-cfg/. "${CFG_DIR}/"
    echo "[mods] Mode cfg bundles synced from /opt/cs2-cfg."
fi
if [[ -n "${CS2_MODE:-}" ]] && [[ -f "${CFG_DIR}/${CS2_MODE}.cfg" ]]; then
    SERVER_CFG="${CFG_DIR}/server.cfg"
    EXEC_LINE="exec ${CS2_MODE}.cfg"
    touch "${SERVER_CFG}"
    # Drop any previous mode exec line we added, then append the current one
    # (idempotent across restarts / mode changes).
    grep -v '^exec .*\.cfg # cs2-server mode$' "${SERVER_CFG}" > "${SERVER_CFG}.tmp" 2>/dev/null || true
    mv "${SERVER_CFG}.tmp" "${SERVER_CFG}" 2>/dev/null || true
    echo "${EXEC_LINE} # cs2-server mode" >> "${SERVER_CFG}"
    echo "[mods] server.cfg set to exec ${CS2_MODE}.cfg."
fi

echo "[mods] pre.sh complete."
