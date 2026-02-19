#!/usr/bin/env bash
set -euo pipefail

STATE_DIR="${VLESS_STATE_DIR:-/etc/vpn}"

mkdir -p "${STATE_DIR}/clients" "${STATE_DIR}/tls" /var/log
chmod 700 "${STATE_DIR}" "${STATE_DIR}/clients" "${STATE_DIR}/tls"

echo "[entrypoint] starting VLESS manager"
exec /app/vpn-server
