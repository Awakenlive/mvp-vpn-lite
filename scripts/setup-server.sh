#!/usr/bin/env bash
set -euo pipefail

DEVICE="${DEVICE:-mvpvpns0}"
SERVER_IP="${SERVER_IP:-10.8.0.1}"
PREFIX="${PREFIX:-24}"
MTU="${MTU:-1400}"
OWNER="${OWNER:-${SUDO_USER:-$USER}}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "setup-server.sh must run as root" >&2
  exit 1
fi

if ! ip link show "$DEVICE" >/dev/null 2>&1; then
  ip tuntap add dev "$DEVICE" mode tun user "$OWNER"
fi

ip addr replace "$SERVER_IP/$PREFIX" dev "$DEVICE"
ip link set dev "$DEVICE" mtu "$MTU" up

echo "configured $DEVICE: $SERVER_IP/$PREFIX mtu=$MTU owner=$OWNER"
