#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib-tun.sh"

DEVICE="${DEVICE:-mvpvpns0}"
SERVER_IP="${SERVER_IP:-10.8.0.1}"
PREFIX="${PREFIX:-24}"
MTU="${MTU:-1400}"
OWNER="${OWNER:-${SUDO_USER:-${USER:-root}}}"
ROUTE="${ROUTE:-}"

tun_require_root "$(basename "$0")"

if ! tun_link_exists "$DEVICE"; then
  tun_run "$IP_BIN" tuntap add dev "$DEVICE" mode tun user "$OWNER"
fi

tun_run "$IP_BIN" addr replace "$SERVER_IP/$PREFIX" dev "$DEVICE"
tun_run "$IP_BIN" link set dev "$DEVICE" mtu "$MTU" up

if [[ -n "$ROUTE" ]]; then
  tun_run "$IP_BIN" route replace "$ROUTE" dev "$DEVICE"
fi

echo "$(tun_status_prefix) $DEVICE: $SERVER_IP/$PREFIX mtu=$MTU route=${ROUTE:-none} owner=$OWNER"
