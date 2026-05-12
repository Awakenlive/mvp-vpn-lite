#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib-tun.sh"

DEVICE="${DEVICE:-mvpvpn0}"
CLIENT_IP="${CLIENT_IP:-10.8.0.2}"
PREFIX="${PREFIX:-24}"
MTU="${MTU:-1400}"
ROUTE="${ROUTE:-10.8.0.1/32}"
OWNER="${OWNER:-${SUDO_USER:-${USER:-root}}}"

tun_require_root "$(basename "$0")"

if ! tun_link_exists "$DEVICE"; then
  tun_run "$IP_BIN" tuntap add dev "$DEVICE" mode tun user "$OWNER"
fi

tun_run "$IP_BIN" addr replace "$CLIENT_IP/$PREFIX" dev "$DEVICE"
tun_run "$IP_BIN" link set dev "$DEVICE" mtu "$MTU" up
tun_run "$IP_BIN" route replace "$ROUTE" dev "$DEVICE"

echo "$(tun_status_prefix) $DEVICE: $CLIENT_IP/$PREFIX mtu=$MTU route=$ROUTE owner=$OWNER"
