# Testing

Use a writable Go cache when running in restricted environments:

```sh
GOCACHE=/tmp/mvp-vpn-lite-gocache GOMODCACHE=/tmp/mvp-vpn-lite-gomodcache go test ./...
./scripts/check-tun-scripts.sh
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
- Server TUN session path selection and device forwarding helpers.
- Packet stats counters and formatting.
- TLS config loading for server cert/key files and client CA files.
- TUN helper command rendering through `DRY_RUN=1`.

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

## Known test gaps

- No automated integration test opens a real TUN device, because that requires
  root or `CAP_NET_ADMIN`.
- No real network fault injection or end-to-end reconnect test yet.
- No automated full TUN-to-TUN integration test yet.
- No NAT or packet policy tests yet.
