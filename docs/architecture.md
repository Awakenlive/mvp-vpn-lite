# Architecture

The project is split into small packages so each MVP layer can be tested
without opening sockets or TUN devices.

## Components

`internal/packet`

Parses and builds the IPv4 ICMP echo packets used by the demo. It validates
basic IPv4 structure, protocol number, total length, and ICMP echo fields.

`internal/quictransport`

Owns QUIC client/server behavior and packet framing. The default server accepts
one stream per QUIC connection and replies to framed ICMP echo requests. The
server also has a TUN mode that forwards framed packets into a Linux TUN device
and returns packets read from that device. The client has two modes:

- Synthetic mode builds ICMP echo requests in memory and validates replies.
- TUN mode reads raw packets from a Linux TUN device and writes returned packets
  back to that device.

`internal/multipath`

Provides the path scheduler. The current scheduler is round-robin over the
connected QUIC paths.

`internal/tun`

Wraps Linux TUN creation/opening. The device uses `IFF_TUN | IFF_NO_PI`, so the
packet stream contains raw IP packets without a packet-info prefix.

`internal/envconfig`

Reads typed environment defaults for command-line flags. The commands still use
Go's standard `flag` package, so explicit flags override environment-provided
defaults.

`scripts`

Provides idempotent Linux TUN setup/cleanup helpers for client and server
hosts. The helpers can print their `iproute2` commands with `DRY_RUN=1`, and the
server helper can install an optional route through the server TUN device.

`examples`

Contains environment files and systemd units for running the client and server
as long-lived Linux services.

`internal/stats`

Tracks RX/TX packet and byte counts plus drops and errors. The command line
tools can log snapshots periodically with `-stats-interval`, can render those
snapshots as JSON with `-stats-json`, and always log a final snapshot on
shutdown.

`internal/buildinfo`

Stores build-time version metadata used by the `-version` flags in both
commands.

`cmd/server`

Parses flags, creates a `quictransport.ServerConfig`, and starts one or two QUIC
listeners. With `-tun`, it starts the server-side TUN packet pump instead of the
synthetic ICMP responder. Server flags can be defaulted with `MVPVPN_SERVER_*`
environment variables.

`cmd/client`

Parses flags, starts synthetic mode by default, or starts the TUN packet pump
when `-tun` is set. Client flags can be defaulted with `MVPVPN_CLIENT_*`
environment variables.

## Packet flow

Synthetic demo flow:

1. Client builds an IPv4 ICMP echo request from `client-ip` to `virtual-ip`.
2. Round-robin selects a QUIC path.
3. Client writes one length-prefixed packet frame.
4. Server parses the packet and builds an ICMP echo reply.
5. Server writes the reply as another packet frame.
6. Client reads and validates the reply.

Client TUN plus synthetic server flow:

1. Linux routing sends packets for the configured route to `mvpvpn0`.
2. The client reads packets from the TUN file descriptor.
3. Round-robin selects a QUIC path and forwards the packet frame.
4. The server handles supported ICMP echo requests and replies.
5. The client writes returned packet frames back into the TUN device.

TUN-to-TUN flow:

1. The client reads raw IPv4 packets from `mvpvpn0`.
2. Round-robin selects a client QUIC path and forwards the packet frame.
3. The server writes the frame payload into `mvpvpns0`.
4. The server reads response packets from `mvpvpns0`.
5. Round-robin selects an active server QUIC path and forwards the response.
6. The client writes the response back into `mvpvpn0`.

## Concurrency model

The server starts one goroutine per configured listener and one handler
goroutine per accepted connection. Each connection currently accepts one stream.

The TUN client starts one goroutine for reading from the TUN device and one
receiver goroutine per connected path. Writes back to the TUN device are guarded
by a mutex because multiple path receivers can return packets concurrently.
Closed or failed client streams are removed from the active path set, and
outbound packets are retried on the next live path when a write fails. One
reconnect goroutine per configured path watches the active set and redials
missing paths with bounded backoff. The client enables QUIC keepalive packets so
idle broken paths can be noticed by the connection idle timeout. The path set
also remembers recent write failures and gives a just-failed path a short
cooldown after reconnect when another active path is available.

The TUN server mirrors that shape: one goroutine reads from the server TUN
device, and one receiver goroutine per accepted QUIC stream writes packets into
the device. Server-side sends use round-robin across currently active streams.

When `-tun-allow-cidr` is set, TUN packet reader and receiver paths call the
same IPv4 policy helper before forwarding a raw packet. Denied packets are
counted as drops and are not written to QUIC streams or TUN devices.

Cancellation is driven by `context.Context`. When the context is canceled, the
TUN file descriptor and QUIC listeners are closed to unblock pending reads.

## Security notes

This is an MVP transport demo, not production VPN software. The server can load
a configured certificate/key pair, the client can verify a configured CA
certificate, and the server can require client certificates through `-client-ca`.
If no CA is configured, the client falls back to insecure demo mode for the
server's ephemeral self-signed certificate. Production use would still need
key rotation, replay/abuse controls, stronger authorization rules, and
deployment-specific routing/NAT policy.
