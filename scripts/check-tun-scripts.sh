#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

bash -n \
  "$SCRIPT_DIR/lib-tun.sh" \
  "$SCRIPT_DIR/setup-client.sh" \
  "$SCRIPT_DIR/setup-server.sh" \
  "$SCRIPT_DIR/cleanup-client.sh" \
  "$SCRIPT_DIR/cleanup-server.sh" \
  "$SCRIPT_DIR/integration-root.sh"

DRY_RUN=1 "$SCRIPT_DIR/setup-server.sh" >/dev/null
DRY_RUN=1 "$SCRIPT_DIR/setup-client.sh" >/dev/null
DRY_RUN=1 "$SCRIPT_DIR/cleanup-client.sh" >/dev/null
DRY_RUN=1 "$SCRIPT_DIR/cleanup-server.sh" >/dev/null

DRY_RUN=1 ROUTE=10.8.0.2/32 "$SCRIPT_DIR/setup-server.sh" >/dev/null
DRY_RUN=1 ROUTE=10.8.0.2/32 "$SCRIPT_DIR/cleanup-server.sh" >/dev/null

echo "TUN helper script checks passed"
