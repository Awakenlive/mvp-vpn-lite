#!/usr/bin/env bash
set -euo pipefail

DEVICE="${DEVICE:-mvpvpn0}"
CLIENT_IP="${CLIENT_IP:-10.8.0.2}"
PREFIX="${PREFIX:-24}"
MTU="${MTU:-1400}"
ROUTE="${ROUTE:-10.8.0.1/32}"
OWNER="${OWNER:-${SUDO_USER:-$USER}}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "setup-client.sh must run as root" >&2
  exit 1
fi

if ! ip link show "$DEVICE" >/dev/null 2>&1; then
  ip tuntap add dev "$DEVICE" mode tun user "$OWNER"
fi

ip addr replace "$CLIENT_IP/$PREFIX" dev "$DEVICE"
ip link set dev "$DEVICE" mtu "$MTU" up
ip route replace "$ROUTE" dev "$DEVICE"

echo "configured $DEVICE: $CLIENT_IP/$PREFIX mtu=$MTU route=$ROUTE owner=$OWNER"
