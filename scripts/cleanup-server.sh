#!/usr/bin/env bash
set -euo pipefail

DEVICE="${DEVICE:-mvpvpns0}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "cleanup-server.sh must run as root" >&2
  exit 1
fi

if ip link show "$DEVICE" >/dev/null 2>&1; then
  ip link set dev "$DEVICE" down >/dev/null 2>&1 || true
  ip link delete "$DEVICE" >/dev/null 2>&1 || true
fi

echo "removed $DEVICE"
