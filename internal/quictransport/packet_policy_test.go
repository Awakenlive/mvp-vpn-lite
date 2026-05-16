package quictransport

import (
	"net"
	"testing"

	"mvp-vpn-lite/internal/packet"
)

func TestPacketPolicyAllowsEmptyPolicy(t *testing.T) {
	t.Parallel()

	rawPacket, err := packet.BuildICMPEchoRequest(net.IPv4(192, 0, 2, 10), net.IPv4(10, 8, 0, 1), 1, 1, nil)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	if !(packetPolicy{}).allow(rawPacket) {
		t.Fatal("empty packet policy denied packet")
	}
}

func TestPacketPolicyAllowsPacketInsideCIDR(t *testing.T) {
	t.Parallel()

	policy, err := newPacketPolicy("10.8.0.0/24")
	if err != nil {
		t.Fatalf("newPacketPolicy() error = %v", err)
	}
	rawPacket, err := packet.BuildICMPEchoRequest(net.IPv4(10, 8, 0, 2), net.IPv4(10, 8, 0, 1), 1, 1, nil)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	if !policy.allow(rawPacket) {
		t.Fatal("packet policy denied packet inside CIDR")
	}
}

func TestPacketPolicyDeniesPacketOutsideCIDR(t *testing.T) {
	t.Parallel()

	policy, err := newPacketPolicy("10.8.0.0/24")
	if err != nil {
		t.Fatalf("newPacketPolicy() error = %v", err)
	}
	rawPacket, err := packet.BuildICMPEchoRequest(net.IPv4(192, 0, 2, 10), net.IPv4(10, 8, 0, 1), 1, 1, nil)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	if policy.allow(rawPacket) {
		t.Fatal("packet policy allowed packet outside CIDR")
	}
}

func TestPacketPolicyRejectsIPv6CIDR(t *testing.T) {
	t.Parallel()

	if _, err := newPacketPolicy("2001:db8::/32"); err == nil {
		t.Fatal("newPacketPolicy(IPv6) error = nil, want error")
	}
}

func TestPacketPolicyDeniesMalformedPacket(t *testing.T) {
	t.Parallel()

	policy, err := newPacketPolicy("10.8.0.0/24")
	if err != nil {
		t.Fatalf("newPacketPolicy() error = %v", err)
	}
	if policy.allow([]byte{0x45, 0}) {
		t.Fatal("packet policy allowed malformed packet")
	}
}
