# Routing and NAT

`mvp-vpn-lite` forwards raw IPv4 packets through TUN devices. It does not manage
host-wide forwarding or NAT automatically because those settings are deployment
specific and can affect the whole machine.

## Default overlay

The helper scripts configure this small overlay by default:

```text
client mvpvpn0  10.8.0.2/24  route 10.8.0.1/32 via mvpvpn0
server mvpvpns0 10.8.0.1/24
```

That is enough for `ping 10.8.0.1` from the client side.

## Routing another subnet

If the server host should return traffic to an extra client-side subnet, add a
server-side route through the server TUN device:

```sh
sudo ROUTE=10.8.0.2/32 ./scripts/setup-server.sh
```

For a larger overlay, widen both helper routes and the packet policy:

```sh
sudo ROUTE=10.8.0.0/24 ./scripts/setup-server.sh
MVPVPN_SERVER_TUN_ALLOW_CIDR=10.8.0.0/24
MVPVPN_CLIENT_TUN_ALLOW_CIDR=10.8.0.0/24
```

If `MVPVPN_*_TUN_ALLOW_CIDR` is set, both the source and destination address of
each packet must be inside the CIDR. Leave the policy empty when forwarding
traffic to networks outside the overlay.

## NAT on the server host

To let tunnel clients reach a server-side uplink through source NAT, enable IPv4
forwarding and add a masquerade rule on the server host. Replace `eth0` with the
real uplink interface:

```sh
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE
```

Cleanup:

```sh
sudo iptables -t nat -D POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE
sudo sysctl -w net.ipv4.ip_forward=0
```

For persistent deployments, put equivalent rules into the host firewall manager
already used by the system. The project intentionally keeps those rules out of
the setup helpers so a demo run cannot accidentally change global routing
policy.

## Useful checks

```sh
ip addr show mvpvpn0
ip addr show mvpvpns0
ip route get 10.8.0.1
sysctl net.ipv4.ip_forward
sudo iptables -t nat -S
```
