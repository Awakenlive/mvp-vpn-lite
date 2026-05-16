# mvp-vpn-lite

[![CI](https://github.com/AwakenLive/mvp-vpn-lite/actions/workflows/ci.yml/badge.svg)](https://github.com/AwakenLive/mvp-vpn-lite/actions/workflows/ci.yml)

`mvp-vpn-lite` is a small Go prototype of a packet tunnel over QUIC. It can run
as a synthetic ICMP demo, or pump raw IPv4 packets between Linux TUN devices on
the client and server side.

## Current status

Stage 10 is implemented: both client and server have Linux TUN modes, the TUN
client reconnects failed QUIC paths with bounded backoff, CI and root
integration checks cover the important packet flows, and the command-line tools
can be configured from environment files for systemd-style operation. The demo
client/server still exist for quick socket-only checks.

Implemented pieces:

- IPv4 and ICMP echo packet parsing/building.
- Length-prefixed packet frames over QUIC streams.
- Demo server listening on one or two QUIC addresses.
- Demo client sending synthetic ICMP echo requests without TUN.
- Round-robin scheduling across available paths.
- Linux TUN client/server modes and idempotent setup/cleanup helper scripts.
- RX/TX/drop/error counters with `-stats-interval`.
- Optional JSON stats logs with `-stats-json`.
- Optional server TLS cert/key files, client CA verification, and mTLS client
  certificates.
- TUN client path removal, packet failover, and reconnect with bounded backoff.
- Optional IPv4 TUN packet policy with `-tun-allow-cidr`.
- Environment-variable defaults for every command-line flag.
- Example environment files and systemd units for client/server services.
- GitHub Actions CI plus root-only integration scripts for real TUN, fault
  injection, reconnect, MTU, and soak checks.

Main limitations:

- Linux only for real TUN mode.
- When `-ca-cert` is not provided, the client uses demo TLS mode and skips
  certificate verification for the server's ephemeral self-signed certificate.
- The default server mode still answers IPv4 ICMP echo packets itself.
- TUN server mode forwards raw IPv4 packets to the server TUN device, but there
  is no automatic host-wide NAT management.
- There is no packet retransmission, reordering, or latency-based path scoring
  beyond active/inactive health tracking, short failure cooldown, and reconnect.

## Shape

```text
client app/kernel
      |
      v
  mvpvpn0 TUN
      |
      v
cmd/client -- QUIC path 0 --> cmd/server -- mvpvpns0 TUN -- server kernel
          \- QUIC path 1 -/
```

## Requirements

- Go 1.25 or newer.
- Linux with `/dev/net/tun` for `-tun` mode.
- `iproute2` for the helper scripts.
- Optional: systemd for the example service units.

## Quick demo without TUN

Run the server in one terminal:

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache \
  go run ./cmd/server -listen0 127.0.0.1:44433 -listen1 127.0.0.1:44434
```

Run the synthetic ICMP client in another terminal:

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache \
  go run ./cmd/client -server0 127.0.0.1:44433 -server1 127.0.0.1:44434 -count 4
```

The client should log four echo replies, alternating between path 0 and path 1.

## TLS trust

By default the server generates an ephemeral self-signed certificate and the
client accepts it for local demos. For a trusted run, start the server with a
certificate/key pair and pass the issuing CA certificate, or the self-signed
server certificate, to the client:

```sh
go run ./cmd/server \
  -tls-cert ./certs/server.crt \
  -tls-key ./certs/server.key
```

```sh
go run ./cmd/client \
  -ca-cert ./certs/server.crt \
  -server-name localhost
```

For mutual TLS, require a client certificate on the server and provide the
client cert/key on the client:

```sh
go run ./cmd/server \
  -tls-cert ./certs/server.crt \
  -tls-key ./certs/server.key \
  -client-ca ./certs/client-ca.crt
```

```sh
go run ./cmd/client \
  -ca-cert ./certs/server-ca.crt \
  -server-name server.example.com \
  -client-cert ./certs/client.crt \
  -client-key ./certs/client.key
```

## TUN-to-TUN mode

On the server host, create and configure the server TUN device:

```sh
sudo ./scripts/setup-server.sh
```

The default server setup assigns `10.8.0.1/24` to `mvpvpns0`. If the server
needs an explicit route back to a client-side or downstream network, pass it as
`ROUTE`:

```sh
sudo ROUTE=10.8.0.2/32 ./scripts/setup-server.sh
```

Start the TUN server:

```sh
go run ./cmd/server \
  -tun \
  -tun-name mvpvpns0 \
  -listen0 127.0.0.1:44433 \
  -listen1 127.0.0.1:44434
```

On the client host, create and configure the client TUN device:

```sh
sudo ./scripts/setup-client.sh
```

The client helper assigns `10.8.0.2/24` to `mvpvpn0` and routes `10.8.0.1/32`
through it. Override `DEVICE`, `CLIENT_IP`, `PREFIX`, `MTU`, `ROUTE`, or
`OWNER` as needed.

Start the TUN client:

```sh
go run ./cmd/client \
  -tun \
  -tun-name mvpvpn0 \
  -server0 127.0.0.1:44433 \
  -server1 127.0.0.1:44434
```

Add `-tun-allow-cidr 10.8.0.0/24` on either endpoint to drop raw IPv4 packets
whose source or destination is outside the overlay CIDR.

In another shell, send traffic through the configured route:

```sh
ping 10.8.0.1
```

Clean up the device when finished:

```sh
sudo ./scripts/cleanup-client.sh
sudo ./scripts/cleanup-server.sh
```

To inspect the exact `ip` commands without root or without changing the host,
run the helpers with `DRY_RUN=1`:

```sh
DRY_RUN=1 ./scripts/setup-server.sh
DRY_RUN=1 ./scripts/setup-client.sh
DRY_RUN=1 ./scripts/cleanup-client.sh
DRY_RUN=1 ./scripts/cleanup-server.sh
```

## Useful flags

Every flag can also be set through an environment variable. Explicit flags
override environment defaults. See `docs/operations.md` for the full mapping and
systemd examples.

Server:

- `-listen0`, `-listen1`: QUIC listen addresses. Leave either empty to use one
  path.
- `-tls-cert`, `-tls-key`: PEM certificate and private key files. If omitted,
  the server generates an ephemeral demo certificate.
- `-client-ca`: PEM CA used to verify client certificates for mTLS.
- `-virtual-ip`: virtual server IPv4 address, default `10.8.0.1`.
- `-client-ip`: client tunnel IPv4 address, default `10.8.0.2`.
- `-tun`: enable server TUN packet pump mode.
- `-tun-name`: server TUN device name, default `mvpvpns0`.
- `-tun-allow-cidr`: optional IPv4 CIDR policy for TUN packets.
- `-stats-interval`: periodic stats interval, default `10s`; use `0` to disable
  periodic logs.
- `-stats-json`: write stats snapshots as JSON.
- `-version`: print the binary version and exit.

Client:

- `-server0`, `-server1`: QUIC server addresses. Leave either empty to use one
  path.
- `-ca-cert`: PEM CA/server certificate used to verify the QUIC server. If
  omitted, the client uses demo insecure TLS.
- `-server-name`: TLS server name override for certificate verification.
- `-client-cert`, `-client-key`: PEM client certificate/key for mTLS.
- `-count`: number of synthetic echo requests in non-TUN mode.
- `-tun`: enable TUN packet pump mode.
- `-tun-name`: TUN device name, default `mvpvpn0`.
- `-tun-allow-cidr`: optional IPv4 CIDR policy for TUN packets.
- `-reconnect-min`, `-reconnect-max`: bounded backoff range for reconnecting
  failed TUN paths, defaults `1s` and `30s`.
- `-stats-interval`: periodic stats interval, default `10s`; use `0` to disable
  periodic logs.
- `-stats-json`: write stats snapshots as JSON.
- `-version`: print the binary version and exit.

## Tests

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache go test ./...
go vet ./...
./scripts/check-tun-scripts.sh
./scripts/check-operational-examples.sh
```

More details are in `docs/testing.md`. Operational install notes are in
`docs/operations.md`; routing and NAT notes are in `docs/routing-nat.md`.
