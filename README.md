# mvp-vpn-lite

`mvp-vpn-lite` is a small Go prototype of a packet tunnel over QUIC. It can run
as a synthetic ICMP demo, or pump raw IPv4 packets between Linux TUN devices on
the client and server side.

## Current status

Stage 6 is implemented: both client and server have Linux TUN modes, and the
demo client/server still exist for quick socket-only checks. Packet counters and
periodic stats logging are wired into demo and TUN paths.

Implemented pieces:

- IPv4 and ICMP echo packet parsing/building.
- Length-prefixed packet frames over QUIC streams.
- Demo server listening on one or two QUIC addresses.
- Demo client sending synthetic ICMP echo requests without TUN.
- Round-robin scheduling across available paths.
- Linux TUN client/server modes and setup/cleanup helper scripts.
- RX/TX/drop/error counters with `-stats-interval`.

Main limitations:

- Linux only for real TUN mode.
- Demo TLS uses an ephemeral self-signed certificate and client-side
  `InsecureSkipVerify`.
- The default server mode still answers IPv4 ICMP echo packets itself.
- TUN server mode forwards raw IPv4 packets to the server TUN device, but there
  is no authentication, routing policy, or NAT management yet.

## Requirements

- Go 1.25 or newer.
- Linux with `/dev/net/tun` for `-tun` mode.
- `iproute2` for the helper scripts.

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

## TUN-to-TUN mode

On the server host, create and configure the server TUN device:

```sh
sudo ./scripts/setup-server.sh
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

Start the TUN client:

```sh
go run ./cmd/client \
  -tun \
  -tun-name mvpvpn0 \
  -server0 127.0.0.1:44433 \
  -server1 127.0.0.1:44434
```

In another shell, send traffic through the configured route:

```sh
ping 10.8.0.1
```

Clean up the device when finished:

```sh
sudo ./scripts/cleanup-client.sh
sudo ./scripts/cleanup-server.sh
```

## Useful flags

Server:

- `-listen0`, `-listen1`: QUIC listen addresses. Leave either empty to use one
  path.
- `-virtual-ip`: virtual server IPv4 address, default `10.8.0.1`.
- `-client-ip`: client tunnel IPv4 address, default `10.8.0.2`.
- `-tun`: enable server TUN packet pump mode.
- `-tun-name`: server TUN device name, default `mvpvpns0`.
- `-stats-interval`: periodic stats interval, default `10s`; use `0` to disable
  periodic logs.

Client:

- `-server0`, `-server1`: QUIC server addresses. Leave either empty to use one
  path.
- `-count`: number of synthetic echo requests in non-TUN mode.
- `-tun`: enable TUN packet pump mode.
- `-tun-name`: TUN device name, default `mvpvpn0`.
- `-stats-interval`: periodic stats interval, default `10s`; use `0` to disable
  periodic logs.

## Tests

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache go test ./...
```

More details are in `docs/testing.md`.
