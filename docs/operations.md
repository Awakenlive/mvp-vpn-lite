# Operations

This document collects the practical pieces for running `mvp-vpn-lite` as a
small Linux service. It is still an MVP transport demo, but the commands can be
driven from environment files and systemd units.

## Build

Build separate binaries for the client and server:

```sh
go build -o ./bin/mvp-vpn-lite-server ./cmd/server
go build -o ./bin/mvp-vpn-lite-client ./cmd/client
```

For a system install, copy them to `/usr/local/bin`:

```sh
sudo install -m 0755 ./bin/mvp-vpn-lite-server /usr/local/bin/mvp-vpn-lite-server
sudo install -m 0755 ./bin/mvp-vpn-lite-client /usr/local/bin/mvp-vpn-lite-client
```

The TUN helper scripts used by the example services live under
`/usr/local/share/mvp-vpn-lite/scripts`:

```sh
sudo install -d -m 0755 /usr/local/share/mvp-vpn-lite/scripts
sudo install -m 0755 ./scripts/setup-server.sh ./scripts/cleanup-server.sh \
  ./scripts/setup-client.sh ./scripts/cleanup-client.sh ./scripts/lib-tun.sh \
  /usr/local/share/mvp-vpn-lite/scripts/
```

## Environment Files

Every command-line flag has a matching environment variable. The environment
value becomes the flag default, and an explicit flag still wins.

Server variables:

| Flag | Environment variable |
| --- | --- |
| `-listen0` | `MVPVPN_SERVER_LISTEN0` |
| `-listen1` | `MVPVPN_SERVER_LISTEN1` |
| `-tls-cert` | `MVPVPN_SERVER_TLS_CERT` |
| `-tls-key` | `MVPVPN_SERVER_TLS_KEY` |
| `-client-ca` | `MVPVPN_SERVER_CLIENT_CA` |
| `-virtual-ip` | `MVPVPN_SERVER_VIRTUAL_IP` |
| `-client-ip` | `MVPVPN_SERVER_CLIENT_IP` |
| `-stats-interval` | `MVPVPN_SERVER_STATS_INTERVAL` |
| `-stats-json` | `MVPVPN_SERVER_STATS_JSON` |
| `-tun` | `MVPVPN_SERVER_TUN` |
| `-tun-name` | `MVPVPN_SERVER_TUN_NAME` |
| `-tun-allow-cidr` | `MVPVPN_SERVER_TUN_ALLOW_CIDR` |

Client variables:

| Flag | Environment variable |
| --- | --- |
| `-server0` | `MVPVPN_CLIENT_SERVER0` |
| `-server1` | `MVPVPN_CLIENT_SERVER1` |
| `-ca-cert` | `MVPVPN_CLIENT_CA_CERT` |
| `-server-name` | `MVPVPN_CLIENT_SERVER_NAME` |
| `-client-cert` | `MVPVPN_CLIENT_TLS_CERT` |
| `-client-key` | `MVPVPN_CLIENT_TLS_KEY` |
| `-virtual-ip` | `MVPVPN_CLIENT_VIRTUAL_IP` |
| `-client-ip` | `MVPVPN_CLIENT_CLIENT_IP` |
| `-count` | `MVPVPN_CLIENT_COUNT` |
| `-identifier` | `MVPVPN_CLIENT_IDENTIFIER` |
| `-payload` | `MVPVPN_CLIENT_PAYLOAD` |
| `-timeout` | `MVPVPN_CLIENT_TIMEOUT` |
| `-stats-interval` | `MVPVPN_CLIENT_STATS_INTERVAL` |
| `-stats-json` | `MVPVPN_CLIENT_STATS_JSON` |
| `-tun` | `MVPVPN_CLIENT_TUN` |
| `-tun-name` | `MVPVPN_CLIENT_TUN_NAME` |
| `-tun-allow-cidr` | `MVPVPN_CLIENT_TUN_ALLOW_CIDR` |
| `-reconnect-min` | `MVPVPN_CLIENT_RECONNECT_MIN` |
| `-reconnect-max` | `MVPVPN_CLIENT_RECONNECT_MAX` |

Example environment files are in `examples/env/server.env` and
`examples/env/client.env`. They also include helper-script variables such as
`DEVICE`, `SERVER_IP`, `CLIENT_IP`, `ROUTE`, `MTU`, and `OWNER`.

The examples leave TLS verification in local demo mode. For a trusted setup,
set `MVPVPN_SERVER_TLS_CERT` and `MVPVPN_SERVER_TLS_KEY` on the server, and set
`MVPVPN_CLIENT_CA_CERT` plus `MVPVPN_CLIENT_SERVER_NAME` on the client. For
mutual TLS, also set `MVPVPN_SERVER_CLIENT_CA` on the server and
`MVPVPN_CLIENT_TLS_CERT`/`MVPVPN_CLIENT_TLS_KEY` on the client.

`MVPVPN_*_TUN_ALLOW_CIDR` enables a simple IPv4 packet policy in TUN mode. When
set, both the source and destination IP of each raw IPv4 packet must be inside
the configured CIDR. Leave it empty if the tunnel is meant to carry routed
subnets outside the default `10.8.0.0/24` overlay.

## Systemd

Install and edit the example environment files:

```sh
sudo install -d -m 0755 /etc/mvp-vpn-lite
sudo install -m 0644 ./examples/env/server.env /etc/mvp-vpn-lite/server.env
sudo install -m 0644 ./examples/env/client.env /etc/mvp-vpn-lite/client.env
sudoedit /etc/mvp-vpn-lite/server.env
sudoedit /etc/mvp-vpn-lite/client.env
```

Install the example service units:

```sh
sudo install -m 0644 ./examples/systemd/mvp-vpn-lite-server.service /etc/systemd/system/
sudo install -m 0644 ./examples/systemd/mvp-vpn-lite-client.service /etc/systemd/system/
sudo systemctl daemon-reload
```

Start the server service on the server host:

```sh
sudo systemctl enable --now mvp-vpn-lite-server.service
sudo journalctl -u mvp-vpn-lite-server.service -f
```

Start the client service on the client host:

```sh
sudo systemctl enable --now mvp-vpn-lite-client.service
sudo journalctl -u mvp-vpn-lite-client.service -f
```

The example services run the setup helper before the daemon and the cleanup
helper after it stops. They expect the binaries under `/usr/local/bin` and the
helper scripts under `/usr/local/share/mvp-vpn-lite/scripts`.

## Validation

For a quick service-level check, confirm both units are active and then send a
packet through the client route:

```sh
systemctl is-active mvp-vpn-lite-server.service
systemctl is-active mvp-vpn-lite-client.service
ping 10.8.0.1
```

Useful local checks before installing:

```sh
go test ./...
go vet ./...
./scripts/check-tun-scripts.sh
./scripts/check-operational-examples.sh
```

Root-only integration checks:

```sh
sudo ./scripts/integration-root.sh
sudo ./scripts/integration-mtu.sh
sudo SOAK_SECONDS=60 ./scripts/integration-soak.sh
```

## Troubleshooting

- If the TUN helper fails, run it with `DRY_RUN=1` to inspect the `ip` commands.
- If TLS verification fails, check `MVPVPN_CLIENT_CA_CERT` and
  `MVPVPN_CLIENT_SERVER_NAME` against the server certificate SANs.
- If packets leave the client but no replies return, check the server-side TUN
  route and the `ROUTE` value in `/etc/mvp-vpn-lite/server.env`.
- If a path drops, the client should log reconnect attempts with the configured
  `MVPVPN_CLIENT_RECONNECT_MIN` and `MVPVPN_CLIENT_RECONNECT_MAX` range.
- If packets are unexpectedly dropped, check `MVPVPN_SERVER_TUN_ALLOW_CIDR` and
  `MVPVPN_CLIENT_TUN_ALLOW_CIDR`; both endpoints enforce their own policy.
