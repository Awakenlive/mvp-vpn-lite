# Protocol

The on-stream protocol is intentionally small: each QUIC stream carries a
sequence of length-prefixed raw IPv4 packets.

## QUIC

- ALPN: `mvp-vpn-lite`
- TLS: TLS 1.3
- Server certificate: ephemeral self-signed certificate generated at startup
- Client verification: disabled for the demo

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
TUN device. Any packet filtering, routing, NAT, or policy enforcement is outside
the current MVP.

## Multipath behavior

The client creates path 0 from `server0` and path 1 from `server1` when the
addresses are provided. Packet sends use round-robin path selection across the
connected paths.

There is no retransmission, path health scoring, congestion coordination, or
packet reordering logic in the MVP. Reliability is left to QUIC streams on each
individual path. TUN mode does remove closed streams from the active path set,
and a packet write that fails on one path is retried on the next active path.
