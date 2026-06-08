#!/bin/bash
# Wrapper entrypoint. Two modes:
#
#  Non-shared (default, CS2_SHARED_MODE!=1):
#    Runs as the unprivileged `steam` user. Force-installs our pre/post hooks
#    onto the (per-instance) game volume, then hands off to the base entry.sh,
#    which runs SteamCMD into this instance's own ~40GB game copy.
#
#  Shared (CS2_SHARED_MODE=1):
#    The container is started as root with CAP_SYS_ADMIN by the orchestrator.
#    We mount an OverlayFS combining the shared read-only game copy (lowerdir)
#    with a tiny per-instance writable layer (upperdir), then drop to `steam`
#    and run the base entry.sh. Each instance costs only a few MB instead of a
#    full game copy. Mods + gameinfo are already baked into the shared layer.
set -euo pipefail

STEAMAPPDIR="${STEAMAPPDIR:-/home/steam/cs2-dedicated}"
HOMEDIR="${HOMEDIR:-/home/steam}"

install_hooks() {
    cp -f /opt/cs2-hooks/pre.sh  "${STEAMAPPDIR}/pre.sh"
    cp -f /opt/cs2-hooks/post.sh "${STEAMAPPDIR}/post.sh"
    chmod +x "${STEAMAPPDIR}/pre.sh" "${STEAMAPPDIR}/post.sh"
}

if [[ "${CS2_SHARED_MODE:-0}" != "1" ]]; then
    # ---- Non-shared: original tested path (runs as steam) ----
    mkdir -p "${STEAMAPPDIR}"
    install_hooks
    exec bash "${HOMEDIR}/entry.sh"
fi

# ---- Shared mode (must be root to mount the overlay) ----
if [[ "$(id -u)" != "0" ]]; then
    echo "[overlay] ERROR: shared mode requires the container to run as root with CAP_SYS_ADMIN" >&2
    exit 1
fi

LOWER="${CS2_SHARED_LOWER:-/shared/cs2}"
UPPER="/instance/upper"
WORK="/instance/work"

if [[ ! -f "${LOWER}/.cs2-seeded" ]]; then
    echo "[overlay] ERROR: shared lower ${LOWER} is not seeded (.cs2-seeded missing)" >&2
    exit 1
fi

echo "[overlay] Mounting overlay: lower=${LOWER} upper=${UPPER} -> ${STEAMAPPDIR}"
mkdir -p "${UPPER}" "${WORK}" "${STEAMAPPDIR}"
mount -t overlay overlay \
    -o "lowerdir=${LOWER},upperdir=${UPPER},workdir=${WORK}" \
    "${STEAMAPPDIR}"

# The merged tree must be owned by steam (uid 1000) so the server can write.
chown steam:steam "${STEAMAPPDIR}" "${UPPER}" "${WORK}"

install_hooks
chown steam:steam "${STEAMAPPDIR}/pre.sh" "${STEAMAPPDIR}/post.sh"

echo "[overlay] Dropping to steam and starting server"
# su (not -l) keeps the CS2_* environment while resetting HOME/USER to steam.
exec su steam -c "bash '${HOMEDIR}/entry.sh'"
