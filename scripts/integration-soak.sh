#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
SERVER_BIN="${SERVER_BIN:-/tmp/mvp-vpn-lite-soak-server}"
CLIENT_BIN="${CLIENT_BIN:-/tmp/mvp-vpn-lite-soak-client}"
LOG_DIR="${LOG_DIR:-/tmp/mvp-vpn-lite-soak}"
GOCACHE="${GOCACHE:-/tmp/mvp-vpn-lite-soak-gocache}"
GOMODCACHE="${GOMODCACHE:-/tmp/mvp-vpn-lite-soak-gomodcache}"
SOAK_SECONDS="${SOAK_SECONDS:-60}"
PING_INTERVAL="${PING_INTERVAL:-0.2}"

server_pid=""
client_pid=""

log() {
  printf '[integration-soak] %s\n' "$*"
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
  ip netns del mvpvpn-soak-client >/dev/null 2>&1 || true
  ip netns del mvpvpn-soak-server >/dev/null 2>&1 || true
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
    tail -120 "$file" || true
    fail "assertion failed"
  fi
}

main() {
  if [[ "$(id -u)" -ne 0 ]]; then
    fail "must run as root or with CAP_NET_ADMIN"
  fi
  require_command go
  require_command ip
  require_command ping

  rm -rf "$LOG_DIR"
  mkdir -p "$LOG_DIR" "$GOCACHE" "$GOMODCACHE"
  trap cleanup EXIT

  log "building binaries"
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -buildvcs=false -o "$SERVER_BIN" "$ROOT_DIR/cmd/server"
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -buildvcs=false -o "$CLIENT_BIN" "$ROOT_DIR/cmd/client"

  cleanup

  ip netns add mvpvpn-soak-server
  ip netns add mvpvpn-soak-client
  ip link add veth-soaks type veth peer name veth-soakc
  ip link set veth-soaks netns mvpvpn-soak-server
  ip link set veth-soakc netns mvpvpn-soak-client

  ip netns exec mvpvpn-soak-server ip link set lo up
  ip netns exec mvpvpn-soak-client ip link set lo up
  ip netns exec mvpvpn-soak-server ip addr add 192.0.2.1/24 dev veth-soaks
  ip netns exec mvpvpn-soak-client ip addr add 192.0.2.2/24 dev veth-soakc
  ip netns exec mvpvpn-soak-server ip link set veth-soaks up
  ip netns exec mvpvpn-soak-client ip link set veth-soakc up

  ip netns exec mvpvpn-soak-server env DEVICE=mvpvpns0 SERVER_IP=10.8.0.1 PREFIX=24 MTU=1400 OWNER=root \
    "$ROOT_DIR/scripts/setup-server.sh" >"$LOG_DIR/setup-server.log" 2>&1
  ip netns exec mvpvpn-soak-client env DEVICE=mvpvpn0 CLIENT_IP=10.8.0.2 PREFIX=24 MTU=1400 ROUTE=10.8.0.1/32 OWNER=root \
    "$ROOT_DIR/scripts/setup-client.sh" >"$LOG_DIR/setup-client.log" 2>&1

  ip netns exec mvpvpn-soak-server "$SERVER_BIN" \
    -tun \
    -tun-name mvpvpns0 \
    -listen0 192.0.2.1:49533 \
    -listen1 192.0.2.1:49534 \
    -tun-allow-cidr 10.8.0.0/24 \
    -stats-interval 5s \
    >"$LOG_DIR/server.log" 2>&1 &
  server_pid="$!"
  sleep 1

  ip netns exec mvpvpn-soak-client "$CLIENT_BIN" \
    -tun \
    -tun-name mvpvpn0 \
    -server0 192.0.2.1:49533 \
    -server1 192.0.2.1:49534 \
    -tun-allow-cidr 10.8.0.0/24 \
    -stats-interval 5s \
    >"$LOG_DIR/client.log" 2>&1 &
  client_pid="$!"
  sleep 3

  log "ping soak for ${SOAK_SECONDS}s"
  ip netns exec mvpvpn-soak-client ping -i "$PING_INTERVAL" -w "$SOAK_SECONDS" 10.8.0.1 >"$LOG_DIR/ping.log" 2>&1

  require_grep " 0% packet loss" "$LOG_DIR/ping.log" "zero packet loss during soak"
  require_grep "received TUN packet" "$LOG_DIR/client.log" "client received TUN traffic"
  require_grep "wrote TUN packet" "$LOG_DIR/server.log" "server wrote TUN traffic"

  if grep -R "not pollable\|read TUN packet" "$LOG_DIR" >/dev/null 2>&1; then
    grep -R "not pollable\|read TUN packet" "$LOG_DIR" || true
    fail "unexpected TUN read error"
  fi

  cleanup
  log "PASS"
  log "logs: $LOG_DIR"
}

main "$@"
