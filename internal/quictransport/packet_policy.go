package quictransport

import (
	"fmt"
	"net"
)

type packetPolicy struct {
	allowedCIDR *net.IPNet
}

func newPacketPolicy(allowedCIDR string) (packetPolicy, error) {
	if allowedCIDR == "" {
		return packetPolicy{}, nil
	}

	_, network, err := net.ParseCIDR(allowedCIDR)
	if err != nil {
		return packetPolicy{}, fmt.Errorf("parse TUN allowed CIDR: %w", err)
	}
	if network.IP.To4() == nil {
		return packetPolicy{}, fmt.Errorf("TUN allowed CIDR must be IPv4: %s", allowedCIDR)
	}
	return packetPolicy{allowedCIDR: network}, nil
}

func (p packetPolicy) allow(rawPacket []byte) bool {
	if p.allowedCIDR == nil {
		return true
	}

	sourceIP, destinationIP, ok := ipv4Endpoints(rawPacket)
	if !ok {
		return false
	}
	return p.allowedCIDR.Contains(sourceIP) && p.allowedCIDR.Contains(destinationIP)
}

func ipv4Endpoints(rawPacket []byte) (net.IP, net.IP, bool) {
	if len(rawPacket) < 20 {
		return nil, nil, false
	}
	if rawPacket[0]>>4 != 4 {
		return nil, nil, false
	}
	headerLen := int(rawPacket[0]&0x0f) * 4
	if headerLen < 20 || len(rawPacket) < headerLen {
		return nil, nil, false
	}
	totalLen := int(rawPacket[2])<<8 | int(rawPacket[3])
	if totalLen < headerLen || totalLen > len(rawPacket) {
		return nil, nil, false
	}

	sourceIP := net.IPv4(rawPacket[12], rawPacket[13], rawPacket[14], rawPacket[15])
	destinationIP := net.IPv4(rawPacket[16], rawPacket[17], rawPacket[18], rawPacket[19])
	return sourceIP, destinationIP, true
}
