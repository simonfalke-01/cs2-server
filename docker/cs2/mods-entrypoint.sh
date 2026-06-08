#!/bin/bash
# Wrapper entrypoint that guarantees our pre/post hooks are installed before the
# base image's entry.sh runs, then hands off to it.
#
# The base image only copies /etc/pre.sh into ${STEAMAPPDIR}/pre.sh when the
# file is absent. On a persistent volume an old hook would otherwise win, so we
# force-install ours on every boot. Our hooks live in /opt/cs2-hooks and simply
# delegate to the staged installer logic.
set -euo pipefail

STEAMAPPDIR="${STEAMAPPDIR:-/home/steam/cs2-dedicated}"

mkdir -p "${STEAMAPPDIR}"

# Force our hooks into place (overrides any stale copies on the volume).
cp -f /opt/cs2-hooks/pre.sh  "${STEAMAPPDIR}/pre.sh"
cp -f /opt/cs2-hooks/post.sh "${STEAMAPPDIR}/post.sh"
chmod +x "${STEAMAPPDIR}/pre.sh" "${STEAMAPPDIR}/post.sh"

# Hand off to the base image's real entrypoint.
exec bash "${HOMEDIR:-/home/steam}/entry.sh"
