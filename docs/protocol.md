# Protocol

The on-stream protocol is intentionally small: each QUIC stream carries a
sequence of length-prefixed raw IPv4 packets.

## QUIC

- ALPN: `mvp-vpn-lite`
- TLS: TLS 1.3
- Server certificate: either configured with `-tls-cert`/`-tls-key`, or an
  ephemeral self-signed certificate generated at startup
- Client verification: enabled when `-ca-cert` is provided; otherwise disabled
  for local demo mode

Each configured path is a separate QUIC connection. The client opens one stream
on each connection and sends packet frames on that stream. In server TUN mode,
the server can also send raw packet frames back on any active stream.

## Packet frame

Every frame is:

```text
0                   1                   2                   3
0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        length (uint32)                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                  raw IPv4 packet bytes ...                    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

- `length` is big-endian.
- `length` must be greater than zero.
- `length` must be no larger than `65535`.
- The payload is copied as raw packet bytes. In TUN mode this is the packet read
  from the Linux TUN device.

## Demo ICMP responder mode

The default server mode only responds to IPv4 ICMP echo requests:

- IPv4 version must be 4.
- Minimum IPv4 header length is 20 bytes.
- IPv4 protocol must be 1, ICMP.
- The destination IP must match the configured virtual server IP.
- ICMP type must be echo request, type 8.

For accepted requests the server returns:

- Source IP set to the configured virtual server IP.
- Destination IP set to the configured client IP.
- ICMP type set to echo reply, type 0.
- Identifier, sequence, and payload copied from the request.
- IPv4 and ICMP checksums recalculated.

Unsupported or malformed packets are dropped and counted as drops in server
stats.

## TUN mode

In TUN mode, the QUIC frame payload is treated as a raw IPv4 packet. The client
and server do not inspect or rewrite the packet before writing it to their local
TUN device. The helper scripts configure the local TUN addresses and routes
needed for the default `10.8.0.1`/`10.8.0.2` check, including an optional
server-side route. Packet filtering, NAT, and policy enforcement are outside the
current MVP.

## Multipath behavior

The client creates path 0 from `server0` and path 1 from `server1` when the
addresses are provided. Packet sends use round-robin path selection across the
connected paths.

TUN mode removes closed or failed streams from the active path set, retries a
packet write on the next active path, and redials missing configured paths with
bounded exponential backoff. If all paths are down, outbound TUN packets are
dropped until at least one path reconnects.

There is no packet retransmission above QUIC, path quality scoring, congestion
coordination, or packet reordering logic in the MVP. Reliability is left to QUIC
streams on each individual path.
