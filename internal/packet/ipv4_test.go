package packet

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestParseIPv4Packet(t *testing.T) {
	t.Parallel()

	request := buildTestEchoRequest(t, net.IPv4(10, 8, 0, 2), net.IPv4(10, 8, 0, 1), 0x1234, 7, []byte("hello"))

	packet, err := ParseIPv4Packet(request)
	if err != nil {
		t.Fatalf("ParseIPv4Packet() error = %v", err)
	}

	if packet.Version != 4 {
		t.Fatalf("Version = %d, want 4", packet.Version)
	}
	if packet.IHL != 20 {
		t.Fatalf("IHL = %d, want 20", packet.IHL)
	}
	if packet.TotalLength != len(request) {
		t.Fatalf("TotalLength = %d, want %d", packet.TotalLength, len(request))
	}
	if packet.Protocol != IPv4ProtocolICMP {
		t.Fatalf("Protocol = %d, want %d", packet.Protocol, IPv4ProtocolICMP)
	}
	if !packet.SourceIP.Equal(net.IPv4(10, 8, 0, 2)) {
		t.Fatalf("SourceIP = %s, want 10.8.0.2", packet.SourceIP)
	}
	if !packet.DestinationIP.Equal(net.IPv4(10, 8, 0, 1)) {
		t.Fatalf("DestinationIP = %s, want 10.8.0.1", packet.DestinationIP)
	}
	if string(packet.Payload[8:]) != "hello" {
		t.Fatalf("Payload suffix = %q, want hello", packet.Payload[8:])
	}
}

func TestBuildICMPEchoReply(t *testing.T) {
	t.Parallel()

	serverIP := net.IPv4(10, 8, 0, 1)
	clientIP := net.IPv4(10, 8, 0, 2)
	request := buildTestEchoRequest(t, clientIP, serverIP, 0xbeef, 42, []byte("ping-data"))

	reply, err := BuildICMPEchoReply(request, serverIP, clientIP)
	if err != nil {
		t.Fatalf("BuildICMPEchoReply() error = %v", err)
	}

	if Checksum(reply[:20]) != 0 {
		t.Fatalf("IPv4 checksum validation = %#04x, want 0", Checksum(reply[:20]))
	}

	ipPacket, err := ParseIPv4Packet(reply)
	if err != nil {
		t.Fatalf("ParseIPv4Packet(reply) error = %v", err)
	}
	if !ipPacket.SourceIP.Equal(serverIP) {
		t.Fatalf("reply source = %s, want %s", ipPacket.SourceIP, serverIP)
	}
	if !ipPacket.DestinationIP.Equal(clientIP) {
		t.Fatalf("reply destination = %s, want %s", ipPacket.DestinationIP, clientIP)
	}

	if Checksum(ipPacket.Payload) != 0 {
		t.Fatalf("ICMP checksum validation = %#04x, want 0", Checksum(ipPacket.Payload))
	}

	icmpPacket, err := ParseICMPPacket(ipPacket.Payload)
	if err != nil {
		t.Fatalf("ParseICMPPacket(reply payload) error = %v", err)
	}
	if icmpPacket.Type != ICMPTypeEchoReply {
		t.Fatalf("ICMP type = %d, want %d", icmpPacket.Type, ICMPTypeEchoReply)
	}
	if icmpPacket.Code != 0 {
		t.Fatalf("ICMP code = %d, want 0", icmpPacket.Code)
	}
	if icmpPacket.Identifier != 0xbeef {
		t.Fatalf("identifier = %#04x, want 0xbeef", icmpPacket.Identifier)
	}
	if icmpPacket.Sequence != 42 {
		t.Fatalf("sequence = %d, want 42", icmpPacket.Sequence)
	}
	if string(icmpPacket.Payload) != "ping-data" {
		t.Fatalf("payload = %q, want ping-data", icmpPacket.Payload)
	}
}

func buildTestEchoRequest(t *testing.T, srcIP, dstIP net.IP, identifier, sequence uint16, payload []byte) []byte {
	t.Helper()

	srcIPv4 := srcIP.To4()
	if srcIPv4 == nil {
		t.Fatalf("source IP must be IPv4")
	}
	dstIPv4 := dstIP.To4()
	if dstIPv4 == nil {
		t.Fatalf("destination IP must be IPv4")
	}

	icmpPayload := make([]byte, 8+len(payload))
	icmpPayload[0] = ICMPTypeEchoRequest
	icmpPayload[1] = 0
	binary.BigEndian.PutUint16(icmpPayload[4:6], identifier)
	binary.BigEndian.PutUint16(icmpPayload[6:8], sequence)
	copy(icmpPayload[8:], payload)
	binary.BigEndian.PutUint16(icmpPayload[2:4], Checksum(icmpPayload))

	packet := make([]byte, 20+len(icmpPayload))
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	packet[8] = 64
	packet[9] = IPv4ProtocolICMP
	copy(packet[12:16], srcIPv4)
	copy(packet[16:20], dstIPv4)
	binary.BigEndian.PutUint16(packet[10:12], Checksum(packet[:20]))
	copy(packet[20:], icmpPayload)

	return packet
}
