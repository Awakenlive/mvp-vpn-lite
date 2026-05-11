#!/usr/bin/env bash
set -euo pipefail

DEVICE="${DEVICE:-mvpvpn0}"
ROUTE="${ROUTE:-10.8.0.1/32}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "cleanup-client.sh must run as root" >&2
  exit 1
fi

ip route del "$ROUTE" dev "$DEVICE" >/dev/null 2>&1 || true

if ip link show "$DEVICE" >/dev/null 2>&1; then
  ip link set dev "$DEVICE" down >/dev/null 2>&1 || true
  ip link delete "$DEVICE" >/dev/null 2>&1 || true
fi

echo "removed $DEVICE"
