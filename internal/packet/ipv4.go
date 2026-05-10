package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const (
	IPv4ProtocolICMP uint8 = 1

	ipv4MinHeaderLen = 20
	ipv4DefaultTTL   = 64
)

// IPv4Packet contains the IPv4 header fields needed by the MVP.
type IPv4Packet struct {
	Version       int
	IHL           int
	TotalLength   int
	Protocol      uint8
	SourceIP      net.IP
	DestinationIP net.IP
	Payload       []byte
}

// ParseIPv4Packet parses an IPv4 packet carrying ICMP.
func ParseIPv4Packet(data []byte) (*IPv4Packet, error) {
	if len(data) < ipv4MinHeaderLen {
		return nil, errors.New("ipv4 packet too short")
	}

	version := int(data[0] >> 4)
	if version != 4 {
		return nil, fmt.Errorf("unsupported ip version %d", version)
	}

	headerLen := int(data[0]&0x0f) * 4
	if headerLen < ipv4MinHeaderLen {
		return nil, fmt.Errorf("invalid ipv4 header length %d", headerLen)
	}
	if len(data) < headerLen {
		return nil, errors.New("ipv4 packet shorter than header length")
	}

	totalLen := int(binary.BigEndian.Uint16(data[2:4]))
	if totalLen < headerLen {
		return nil, fmt.Errorf("invalid ipv4 total length %d", totalLen)
	}
	if totalLen > len(data) {
		return nil, errors.New("ipv4 packet shorter than total length")
	}

	protocol := data[9]
	if protocol != IPv4ProtocolICMP {
		return nil, fmt.Errorf("unsupported ipv4 protocol %d", protocol)
	}

	return &IPv4Packet{
		Version:       version,
		IHL:           headerLen,
		TotalLength:   totalLen,
		Protocol:      protocol,
		SourceIP:      append(net.IP(nil), data[12:16]...),
		DestinationIP: append(net.IP(nil), data[16:20]...),
		Payload:       append([]byte(nil), data[headerLen:totalLen]...),
	}, nil
}

// BuildICMPEchoReply builds an IPv4 ICMP echo reply for a raw echo request.
func BuildICMPEchoReply(request []byte, virtualServerIP net.IP, clientIP net.IP) ([]byte, error) {
	serverIPv4 := virtualServerIP.To4()
	if serverIPv4 == nil {
		return nil, errors.New("virtual server IP must be IPv4")
	}
	clientIPv4 := clientIP.To4()
	if clientIPv4 == nil {
		return nil, errors.New("client IP must be IPv4")
	}

	ipPacket, err := ParseIPv4Packet(request)
	if err != nil {
		return nil, err
	}
	if !ipPacket.DestinationIP.Equal(serverIPv4) {
		return nil, fmt.Errorf("icmp request destination %s does not match virtual server IP %s", ipPacket.DestinationIP, serverIPv4)
	}

	icmpPacket, err := ParseICMPPacket(ipPacket.Payload)
	if err != nil {
		return nil, err
	}
	if icmpPacket.Type != ICMPTypeEchoRequest {
		return nil, fmt.Errorf("icmp packet is type %d, expected echo request", icmpPacket.Type)
	}

	icmpReply := make([]byte, 8+len(icmpPacket.Payload))
	icmpReply[0] = ICMPTypeEchoReply
	icmpReply[1] = 0
	binary.BigEndian.PutUint16(icmpReply[4:6], icmpPacket.Identifier)
	binary.BigEndian.PutUint16(icmpReply[6:8], icmpPacket.Sequence)
	copy(icmpReply[8:], icmpPacket.Payload)
	binary.BigEndian.PutUint16(icmpReply[2:4], Checksum(icmpReply))

	totalLen := ipv4MinHeaderLen + len(icmpReply)
	if totalLen > 0xffff {
		return nil, errors.New("icmp echo reply too large for ipv4")
	}

	reply := make([]byte, totalLen)
	reply[0] = 0x45
	reply[1] = 0
	binary.BigEndian.PutUint16(reply[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(reply[4:6], 0)
	binary.BigEndian.PutUint16(reply[6:8], 0)
	reply[8] = ipv4DefaultTTL
	reply[9] = IPv4ProtocolICMP
	copy(reply[12:16], serverIPv4)
	copy(reply[16:20], clientIPv4)
	binary.BigEndian.PutUint16(reply[10:12], Checksum(reply[:ipv4MinHeaderLen]))
	copy(reply[ipv4MinHeaderLen:], icmpReply)

	return reply, nil
}
