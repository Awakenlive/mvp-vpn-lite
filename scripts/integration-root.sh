#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
SERVER_BIN="${SERVER_BIN:-/tmp/mvp-vpn-lite-integration-server}"
CLIENT_BIN="${CLIENT_BIN:-/tmp/mvp-vpn-lite-integration-client}"
LOG_DIR="${LOG_DIR:-/tmp/mvp-vpn-lite-integration}"
GOCACHE="${GOCACHE:-/tmp/mvp-vpn-lite-integration-gocache}"
GOMODCACHE="${GOMODCACHE:-/tmp/mvp-vpn-lite-integration-gomodcache}"

server_pid=""
client_pid=""
ping_pid=""

log() {
  printf '[integration-root] %s\n' "$*"
}

fail() {
  log "FAILED: $*"
  exit 1
}

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    fail "must run as root or with CAP_NET_ADMIN"
  fi
}

require_command() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || fail "missing required command: $cmd"
}

stop_pid() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
}

cleanup_default_namespace() {
  stop_pid "$ping_pid"
  stop_pid "$client_pid"
  stop_pid "$server_pid"
  ping_pid=""
  client_pid=""
  server_pid=""

  iptables -D OUTPUT -p udp --dport 49234 -j DROP >/dev/null 2>&1 || true
  DEVICE=mvpvpn0 ROUTE=10.8.0.1/32 "$ROOT_DIR/scripts/cleanup-client.sh" >/dev/null 2>&1 || true
  DEVICE=mvpvpns0 "$ROOT_DIR/scripts/cleanup-server.sh" >/dev/null 2>&1 || true
}

cleanup_netns() {
  stop_pid "$ping_pid"
  stop_pid "$client_pid"
  stop_pid "$server_pid"
  ping_pid=""
  client_pid=""
  server_pid=""

  ip netns del mvpvpn-client >/dev/null 2>&1 || true
  ip netns del mvpvpn-server >/dev/null 2>&1 || true
}

cleanup_all() {
  cleanup_default_namespace
  cleanup_netns
}

require_grep() {
  local pattern="$1"
  local file="$2"
  local description="$3"

  if ! grep -q -- "$pattern" "$file"; then
    log "missing ${description}: ${pattern}"
    log "--- ${file} tail ---"
    tail -120 "$file" || true
    fail "assertion failed"
  fi
}

assert_no_processes_or_links_left() {
  if ps -eo args | grep -E '/tmp/mvp-vpn-lite-integration-(client|server)' | grep -v grep >/dev/null; then
    ps -eo pid,args | grep -E '/tmp/mvp-vpn-lite-integration-(client|server)' | grep -v grep || true
    fail "integration client/server process left running"
  fi
  if ip link show mvpvpn0 >/dev/null 2>&1 || ip link show mvpvpns0 >/dev/null 2>&1; then
    fail "default namespace TUN link left after cleanup"
  fi
  if ip netns list | grep -qE '^mvpvpn-(client|server)\b'; then
    ip netns list
    fail "integration network namespace left after cleanup"
  fi
  if iptables -C OUTPUT -p udp --dport 49234 -j DROP >/dev/null 2>&1; then
    fail "temporary iptables fault-injection rule left after cleanup"
  fi
}

build_binaries() {
  log "building binaries"
  mkdir -p "$LOG_DIR" "$GOCACHE" "$GOMODCACHE"
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -buildvcs=false -o "$SERVER_BIN" "$ROOT_DIR/cmd/server"
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -buildvcs=false -o "$CLIENT_BIN" "$ROOT_DIR/cmd/client"
}

test_client_tun_with_synthetic_server() {
  log "real client TUN + synthetic server"
  cleanup_default_namespace

  "$SERVER_BIN" \
    -listen0 127.0.0.1:49133 \
    -listen1 127.0.0.1:49134 \
    -stats-interval 1s \
    >"$LOG_DIR/client-tun-server.log" 2>&1 &
  server_pid="$!"
  sleep 1

  DEVICE=mvpvpn0 CLIENT_IP=10.8.0.2 PREFIX=24 MTU=1400 ROUTE=10.8.0.1/32 OWNER=root \
    "$ROOT_DIR/scripts/setup-client.sh" >"$LOG_DIR/client-tun-setup.log" 2>&1

  "$CLIENT_BIN" \
    -tun \
    -tun-name mvpvpn0 \
    -server0 127.0.0.1:49133 \
    -server1 127.0.0.1:49134 \
    -stats-interval 1s \
    >"$LOG_DIR/client-tun-client.log" 2>&1 &
  client_pid="$!"
  sleep 2

  ping -c 5 -W 1 10.8.0.1 >"$LOG_DIR/client-tun-ping.log" 2>&1

  require_grep "5 packets transmitted, 5 received, 0% packet loss" "$LOG_DIR/client-tun-ping.log" "client TUN ping success"
  require_grep "path 0 connected" "$LOG_DIR/client-tun-client.log" "path 0 connection"
  require_grep "path 1 connected" "$LOG_DIR/client-tun-client.log" "path 1 connection"
  require_grep "received TUN packet" "$LOG_DIR/client-tun-client.log" "TUN reply reception"

  cleanup_default_namespace
}

test_fault_injection_and_reconnect() {
  log "network fault injection + reconnect"
  cleanup_default_namespace

  "$SERVER_BIN" \
    -listen0 127.0.0.1:49233 \
    -listen1 127.0.0.1:49234 \
    -stats-interval 1s \
    >"$LOG_DIR/reconnect-server.log" 2>&1 &
  server_pid="$!"
  sleep 1

  DEVICE=mvpvpn0 CLIENT_IP=10.8.0.2 PREFIX=24 MTU=1400 ROUTE=10.8.0.1/32 OWNER=root \
    "$ROOT_DIR/scripts/setup-client.sh" >"$LOG_DIR/reconnect-setup.log" 2>&1

  "$CLIENT_BIN" \
    -tun \
    -tun-name mvpvpn0 \
    -server0 127.0.0.1:49233 \
    -server1 127.0.0.1:49234 \
    -reconnect-min 200ms \
    -reconnect-max 2s \
    -stats-interval 1s \
    >"$LOG_DIR/reconnect-client.log" 2>&1 &
  client_pid="$!"
  sleep 2

  ping -c 5 -W 1 10.8.0.1 >"$LOG_DIR/reconnect-ping-before.log" 2>&1
  ping -i 0.1 10.8.0.1 >"$LOG_DIR/reconnect-ping-during.log" 2>&1 &
  ping_pid="$!"

  sleep 1
  iptables -I OUTPUT -p udp --dport 49234 -j DROP
  sleep 4
  iptables -D OUTPUT -p udp --dport 49234 -j DROP
  sleep 6
  stop_pid "$ping_pid"
  ping_pid=""

  require_grep "5 packets transmitted, 5 received, 0% packet loss" "$LOG_DIR/reconnect-ping-before.log" "pre-fault ping success"
  require_grep "path 1 receive loop stopped" "$LOG_DIR/reconnect-client.log" "path removal after injected fault"
  require_grep "path 1 reconnect" "$LOG_DIR/reconnect-client.log" "reconnect attempt logging"
  require_grep "path 1 reconnected" "$LOG_DIR/reconnect-client.log" "reconnect success"
  require_grep "path 0 received TUN packet" "$LOG_DIR/reconnect-client.log" "path 0 traffic during fault"

  cleanup_default_namespace
}

test_full_tun_to_tun_netns() {
  log "full TUN-to-TUN in network namespaces"
  cleanup_netns

  ip netns add mvpvpn-server
  ip netns add mvpvpn-client
  ip link add veth-mvps type veth peer name veth-mvpc
  ip link set veth-mvps netns mvpvpn-server
  ip link set veth-mvpc netns mvpvpn-client

  ip netns exec mvpvpn-server ip link set lo up
  ip netns exec mvpvpn-client ip link set lo up
  ip netns exec mvpvpn-server ip addr add 192.0.2.1/24 dev veth-mvps
  ip netns exec mvpvpn-client ip addr add 192.0.2.2/24 dev veth-mvpc
  ip netns exec mvpvpn-server ip link set veth-mvps up
  ip netns exec mvpvpn-client ip link set veth-mvpc up

  ip netns exec mvpvpn-server env DEVICE=mvpvpns0 SERVER_IP=10.8.0.1 PREFIX=24 MTU=1400 OWNER=root \
    "$ROOT_DIR/scripts/setup-server.sh" >"$LOG_DIR/netns-setup-server.log" 2>&1
  ip netns exec mvpvpn-client env DEVICE=mvpvpn0 CLIENT_IP=10.8.0.2 PREFIX=24 MTU=1400 ROUTE=10.8.0.1/32 OWNER=root \
    "$ROOT_DIR/scripts/setup-client.sh" >"$LOG_DIR/netns-setup-client.log" 2>&1

  ip netns exec mvpvpn-server "$SERVER_BIN" \
    -tun \
    -tun-name mvpvpns0 \
    -listen0 192.0.2.1:49333 \
    -listen1 192.0.2.1:49334 \
    -stats-interval 1s \
    >"$LOG_DIR/netns-server.log" 2>&1 &
  server_pid="$!"
  sleep 1

  ip netns exec mvpvpn-client "$CLIENT_BIN" \
    -tun \
    -tun-name mvpvpn0 \
    -server0 192.0.2.1:49333 \
    -server1 192.0.2.1:49334 \
    -stats-interval 1s \
    >"$LOG_DIR/netns-client.log" 2>&1 &
  client_pid="$!"
  sleep 3

  ip netns exec mvpvpn-client ping -c 20 -W 1 10.8.0.1 >"$LOG_DIR/netns-ping.log" 2>&1

  require_grep "20 packets transmitted, 20 received, 0% packet loss" "$LOG_DIR/netns-ping.log" "full TUN-to-TUN ping success"
  require_grep "path 0 connected" "$LOG_DIR/netns-client.log" "netns path 0 connection"
  require_grep "path 1 connected" "$LOG_DIR/netns-client.log" "netns path 1 connection"
  require_grep "received TUN packet" "$LOG_DIR/netns-client.log" "netns client received packet"
  require_grep "wrote TUN packet" "$LOG_DIR/netns-server.log" "netns server wrote packet to TUN"

  cleanup_netns
}

main() {
  require_root
  require_command go
  require_command ip
  require_command iptables
  require_command ping
  require_command grep

  rm -rf "$LOG_DIR"
  mkdir -p "$LOG_DIR"
  trap cleanup_all EXIT

  build_binaries
  test_client_tun_with_synthetic_server
  test_fault_injection_and_reconnect
  test_full_tun_to_tun_netns
  cleanup_all
  assert_no_processes_or_links_left

  log "PASS"
  log "logs: $LOG_DIR"
}

main "$@"
