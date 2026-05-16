#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

for env_file in "$ROOT_DIR"/examples/env/*.env; do
  bash -n "$env_file"
done

grep -q '^EnvironmentFile=-/etc/mvp-vpn-lite/server.env$' "$ROOT_DIR/examples/systemd/mvp-vpn-lite-server.service"
grep -q '^EnvironmentFile=-/etc/mvp-vpn-lite/client.env$' "$ROOT_DIR/examples/systemd/mvp-vpn-lite-client.service"
grep -q '^ExecStart=/usr/local/bin/mvp-vpn-lite-server$' "$ROOT_DIR/examples/systemd/mvp-vpn-lite-server.service"
grep -q '^ExecStart=/usr/local/bin/mvp-vpn-lite-client$' "$ROOT_DIR/examples/systemd/mvp-vpn-lite-client.service"
grep -q '^ExecStartPre=/usr/local/share/mvp-vpn-lite/scripts/setup-server.sh$' "$ROOT_DIR/examples/systemd/mvp-vpn-lite-server.service"
grep -q '^ExecStartPre=/usr/local/share/mvp-vpn-lite/scripts/setup-client.sh$' "$ROOT_DIR/examples/systemd/mvp-vpn-lite-client.service"
grep -q '^MVPVPN_SERVER_CLIENT_CA=' "$ROOT_DIR/examples/env/server.env"
grep -q '^MVPVPN_SERVER_STATS_JSON=' "$ROOT_DIR/examples/env/server.env"
grep -q '^MVPVPN_SERVER_TUN_ALLOW_CIDR=' "$ROOT_DIR/examples/env/server.env"
grep -q '^MVPVPN_CLIENT_TLS_CERT=' "$ROOT_DIR/examples/env/client.env"
grep -q '^MVPVPN_CLIENT_TLS_KEY=' "$ROOT_DIR/examples/env/client.env"
grep -q '^MVPVPN_CLIENT_STATS_JSON=' "$ROOT_DIR/examples/env/client.env"
grep -q '^MVPVPN_CLIENT_TUN_ALLOW_CIDR=' "$ROOT_DIR/examples/env/client.env"

echo "Operational examples check passed"
