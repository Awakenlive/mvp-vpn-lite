# Testing

Use a writable Go cache when running in restricted environments:

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache go test ./...
./scripts/check-tun-scripts.sh
./scripts/check-operational-examples.sh
```

## What the tests and checks cover

- IPv4 checksum and packet parsing/building.
- ICMP echo request/reply construction.
- QUIC packet frame read/write validation.
- Round-robin path scheduling.
- TUN device name normalization.
- TUN pump helpers with in-memory device and stream fakes.
- Client path failover when a TUN write hits a failed QUIC stream.
- TUN client reconnect backoff, active path replacement, and all-paths-down
  packet drops.
- Client path health cooldown after write failures.
- Server TUN session path selection and device forwarding helpers.
- TUN packet policy allow/drop behavior on client and server forwarding paths.
- Packet stats counters plus text and JSON formatting.
- TLS config loading for server cert/key files, client CA files, and client
  certificate/key pairs.
- A socket-level mTLS client/server smoke test over QUIC.
- TUN helper command rendering through `DRY_RUN=1`.
- Environment-variable parsing for command defaults.
- Operational example shape for env files and systemd units.
- Root integration coverage with real Linux TUN devices, network fault
  injection, end-to-end reconnect, and full TUN-to-TUN traffic inside network
  namespaces.
- MTU and short soak root integration coverage.

## Smoke test without TUN

Start the server:

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache \
  go run ./cmd/server -listen0 127.0.0.1:44433 -listen1 127.0.0.1:44434 -stats-interval 0
```

Run the client:

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache \
  go run ./cmd/client -server0 127.0.0.1:44433 -server1 127.0.0.1:44434 -count 4 -stats-interval 0
```

Expected client behavior:

- It connects to both paths.
- It logs echo replies for sequences 1 through 4.
- Paths alternate 0, 1, 0, 1.
- Final stats show 4 TX packets and 4 RX packets.

## Client TUN plus synthetic server manual check

This mode needs Linux TUN privileges on the client host:

```sh
sudo ./scripts/setup-client.sh
```

Then start server and client:

```sh
go run ./cmd/server -listen0 127.0.0.1:44433 -listen1 127.0.0.1:44434
```

```sh
go run ./cmd/client -tun -tun-name mvpvpn0 -server0 127.0.0.1:44433 -server1 127.0.0.1:44434
```

Generate traffic:

```sh
ping 10.8.0.1
```

Clean up:

```sh
sudo ./scripts/cleanup-client.sh
```

## TUN-to-TUN manual check

The helper scripts support dry runs, which makes the route/device commands
checkable without root:

```sh
DRY_RUN=1 ./scripts/setup-server.sh
DRY_RUN=1 ./scripts/setup-client.sh
DRY_RUN=1 ./scripts/cleanup-client.sh
DRY_RUN=1 ./scripts/cleanup-server.sh
```

On the server host:

```sh
sudo ./scripts/setup-server.sh
go run ./cmd/server -tun -tun-name mvpvpns0 -listen0 127.0.0.1:44433 -listen1 127.0.0.1:44434
```

If the server needs an explicit return route, pass it through `ROUTE`, for
example:

```sh
sudo ROUTE=10.8.0.2/32 ./scripts/setup-server.sh
```

On the client host:

```sh
sudo ./scripts/setup-client.sh
go run ./cmd/client -tun -tun-name mvpvpn0 -server0 127.0.0.1:44433 -server1 127.0.0.1:44434
ping 10.8.0.1
```

Clean up:

```sh
sudo ./scripts/cleanup-client.sh
sudo ./scripts/cleanup-server.sh
```

## Automated root integration check

The root integration script exercises the cases that cannot run in ordinary
unit tests because they need real TUN devices, route changes, iptables, and
network namespaces:

```sh
sudo ./scripts/integration-root.sh
```

It builds temporary client/server binaries under `/tmp`, writes logs to
`/tmp/mvp-vpn-lite-integration`, and fails fast if an expected packet-flow or
reconnect assertion is missing.

Covered scenarios:

- Real client TUN device plus the synthetic ICMP server.
- End-to-end network fault injection by blocking one QUIC UDP path with
  `iptables`.
- Client path removal, traffic continuing on the surviving path, and reconnect
  after the injected fault is removed.
- Full TUN-to-TUN traffic between isolated client/server Linux network
  namespaces.
- Cleanup assertions for temporary processes, TUN links, namespaces, and the
  iptables fault-injection rule.

Expected result:

```text
[integration-root] PASS
```

## MTU and soak checks

The MTU script checks real client TUN forwarding with multiple payload sizes
and `ping -M do`, which catches accidental fragmentation or frame-size
regressions:

```sh
sudo ./scripts/integration-mtu.sh
```

The soak script runs a full TUN-to-TUN setup in network namespaces and sends
continuous ping traffic for `SOAK_SECONDS`:

```sh
sudo SOAK_SECONDS=60 ./scripts/integration-soak.sh
```

Both scripts write logs under `/tmp` and fail if packet loss, TUN read errors,
or missing TUN traffic are observed.

## Remaining gaps

NAT is documented in `docs/routing-nat.md`, but there is no automatic NAT
feature in the product and therefore no automatic NAT behavior test. The root
scripts do verify that temporary firewall rules used for fault injection are
removed and that test TUN devices and namespaces are cleaned up.

Latency-based path quality scoring, packet reordering, and retransmission above
QUIC are also outside the MVP. Current tests cover active/inactive path health,
failure cooldown, path removal, reconnect, and traffic continuing over a
surviving path.
