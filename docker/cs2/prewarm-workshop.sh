#!/bin/bash
# prewarm-workshop.sh — best-effort pre-cache of curated Steam Workshop maps.
#
# WHY: anonymous SteamCMD `+workshop_download_item 730 <id>` is unreliable/blocked
# for CS2 (app 730 rejects anonymous standalone workshop pulls). The dependable
# way to fetch a workshop map without a Steam account/GSLT is to let the CS2
# DEDICATED SERVER download it via `+host_workshop_map <id>`. So we boot the
# server headlessly once per curated id, wait for the on-disk cache to appear,
# then stop it. The cache lands in
#   <game>/bin/linuxsteamrt64/steamapps/workshop/content/730/<id>/
# and is reused by runtime `host_workshop_map` / !map switches.
#
# FAIL-SAFE: this never aborts the caller. Any id that does not cache within the
# timeout is skipped — it will simply download on demand at !map time instead.
#
# NOTE: this boots the game engine and cannot be exercised in CI; validate on a
# real deploy. Tunables (env): CS2_PREWARM_WORKSHOP_IDS, CS2_PREWARM_PORT,
# CS2_PREWARM_TIMEOUT, CS2_PREWARM_DISABLE=1 to skip entirely.

# Curated 1v1/aim pool — keep in sync with plugins/Duel1v1/MapPool.cs workshop ids.
: "${CS2_PREWARM_WORKSHOP_IDS:=3070253702 3084291314 3340432449 3071005299}"
: "${CS2_PREWARM_PORT:=27975}"
: "${CS2_PREWARM_TIMEOUT:=300}"

# prewarm_workshop <game-dir> [ids]
#   <game-dir> is the dir containing cs2.sh (e.g. ${STEAMAPPDIR}/game).
prewarm_workshop() {
    local game_dir="$1"
    local ids="${2:-$CS2_PREWARM_WORKSHOP_IDS}"

    if [[ "${CS2_PREWARM_DISABLE:-0}" == "1" ]]; then
        echo "[prewarm] disabled via CS2_PREWARM_DISABLE; skipping."
        return 0
    fi

    local cs2_sh="${game_dir}/cs2.sh"
    local content="${game_dir}/bin/linuxsteamrt64/steamapps/workshop/content/730"

    if [[ ! -x "$cs2_sh" ]]; then
        echo "[prewarm] ${cs2_sh} not found/executable; skipping workshop prewarm." >&2
        return 0
    fi

    local id waited boot_pid logf
    for id in $ids; do
        if [[ -d "${content}/${id}" ]] && compgen -G "${content}/${id}/*" >/dev/null 2>&1; then
            echo "[prewarm] workshop ${id} already cached; skipping."
            continue
        fi
        echo "[prewarm] fetching workshop ${id} via headless host_workshop_map (timeout ${CS2_PREWARM_TIMEOUT}s)…"
        logf="/tmp/prewarm-${id}.log"
        # New session (setsid) so we can signal the whole process group; `timeout`
        # is a hard ceiling even if our early-stop kill is missed.
        setsid timeout --signal=TERM --kill-after=15 "${CS2_PREWARM_TIMEOUT}" \
            bash -c "cd '${game_dir}' && exec ./cs2.sh -dedicated -insecure -port ${CS2_PREWARM_PORT} +host_workshop_map ${id}" \
            >"$logf" 2>&1 &
        boot_pid=$!
        waited=0
        while kill -0 "$boot_pid" 2>/dev/null; do
            if [[ -d "${content}/${id}" ]] && compgen -G "${content}/${id}/*" >/dev/null 2>&1; then
                echo "[prewarm] workshop ${id} downloaded."
                break
            fi
            sleep 5; waited=$((waited + 5))
            if (( waited >= CS2_PREWARM_TIMEOUT )); then
                echo "[prewarm] WARNING: workshop ${id} not cached after ${waited}s; will download on demand." >&2
                break
            fi
        done
        # Stop the headless server: signal the whole process group (negative pid).
        kill -TERM -"$boot_pid" 2>/dev/null || true
        wait "$boot_pid" 2>/dev/null || true
        rm -f "$logf" 2>/dev/null || true
    done
    echo "[prewarm] workshop prewarm complete."
}
