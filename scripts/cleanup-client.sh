#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib-tun.sh"

DEVICE="${DEVICE:-mvpvpn0}"
ROUTE="${ROUTE:-10.8.0.1/32}"

tun_require_root "$(basename "$0")"

tun_try "$IP_BIN" route del "$ROUTE" dev "$DEVICE"

if [[ "$DRY_RUN" == "1" ]] || tun_link_exists "$DEVICE"; then
  tun_try "$IP_BIN" link set dev "$DEVICE" down
  tun_try "$IP_BIN" link delete "$DEVICE"
fi

echo "$(tun_cleanup_prefix) $DEVICE"
