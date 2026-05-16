#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
SERVER_BIN="${SERVER_BIN:-/tmp/mvp-vpn-lite-mtu-server}"
CLIENT_BIN="${CLIENT_BIN:-/tmp/mvp-vpn-lite-mtu-client}"
LOG_DIR="${LOG_DIR:-/tmp/mvp-vpn-lite-mtu}"
GOCACHE="${GOCACHE:-/tmp/mvp-vpn-lite-mtu-gocache}"
GOMODCACHE="${GOMODCACHE:-/tmp/mvp-vpn-lite-mtu-gomodcache}"
MTU_SIZES="${MTU_SIZES:-64 512 1200 1300 1350}"

server_pid=""
client_pid=""

log() {
  printf '[integration-mtu] %s\n' "$*"
}

fail() {
  log "FAILED: $*"
  exit 1
}

stop_pid() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
}

cleanup() {
  stop_pid "$client_pid"
  stop_pid "$server_pid"
  client_pid=""
  server_pid=""
  DEVICE=mvpvpn0 ROUTE=10.8.0.1/32 "$ROOT_DIR/scripts/cleanup-client.sh" >/dev/null 2>&1 || true
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

require_grep() {
  local pattern="$1"
  local file="$2"
  local description="$3"
  if ! grep -q -- "$pattern" "$file"; then
    log "missing ${description}: ${pattern}"
    tail -80 "$file" || true
    fail "assertion failed"
  fi
}

main() {
  if [[ "$(id -u)" -ne 0 ]]; then
    fail "must run as root or with CAP_NET_ADMIN"
  fi
  require_command go
  require_command ping
  require_command ip

  rm -rf "$LOG_DIR"
  mkdir -p "$LOG_DIR" "$GOCACHE" "$GOMODCACHE"
  trap cleanup EXIT

  log "building binaries"
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -buildvcs=false -o "$SERVER_BIN" "$ROOT_DIR/cmd/server"
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -buildvcs=false -o "$CLIENT_BIN" "$ROOT_DIR/cmd/client"

  cleanup

  "$SERVER_BIN" \
    -listen0 127.0.0.1:49433 \
    -listen1 127.0.0.1:49434 \
    -stats-interval 1s \
    >"$LOG_DIR/server.log" 2>&1 &
  server_pid="$!"
  sleep 1

  DEVICE=mvpvpn0 CLIENT_IP=10.8.0.2 PREFIX=24 MTU=1400 ROUTE=10.8.0.1/32 OWNER=root \
    "$ROOT_DIR/scripts/setup-client.sh" >"$LOG_DIR/setup-client.log" 2>&1

  "$CLIENT_BIN" \
    -tun \
    -tun-name mvpvpn0 \
    -server0 127.0.0.1:49433 \
    -server1 127.0.0.1:49434 \
    -tun-allow-cidr 10.8.0.0/24 \
    -stats-interval 1s \
    >"$LOG_DIR/client.log" 2>&1 &
  client_pid="$!"
  sleep 2

  for size in $MTU_SIZES; do
    log "ping payload size ${size}"
    ping -M do -s "$size" -c 3 -W 1 10.8.0.1 >"$LOG_DIR/ping-${size}.log" 2>&1
    require_grep "3 packets transmitted, 3 received, 0% packet loss" "$LOG_DIR/ping-${size}.log" "MTU ping success for size ${size}"
  done

  if grep -R "not pollable\|read TUN packet" "$LOG_DIR" >/dev/null 2>&1; then
    grep -R "not pollable\|read TUN packet" "$LOG_DIR" || true
    fail "unexpected TUN read error"
  fi

  cleanup
  log "PASS"
  log "logs: $LOG_DIR"
}

main "$@"
