package packet

import (
	"encoding/binary"
	"errors"
)

const (
	ICMPTypeEchoReply   uint8 = 0
	ICMPTypeEchoRequest uint8 = 8
)

// ICMPPacket contains the fields needed by the MVP ICMP echo demo.
type ICMPPacket struct {
	Type       uint8
	Code       uint8
	Checksum   uint16
	Identifier uint16
	Sequence   uint16
	Payload    []byte
}

// ParseICMPPacket parses an ICMP echo packet payload.
func ParseICMPPacket(data []byte) (*ICMPPacket, error) {
	if len(data) < 8 {
		return nil, errors.New("icmp packet too short")
	}

	return &ICMPPacket{
		Type:       data[0],
		Code:       data[1],
		Checksum:   binary.BigEndian.Uint16(data[2:4]),
		Identifier: binary.BigEndian.Uint16(data[4:6]),
		Sequence:   binary.BigEndian.Uint16(data[6:8]),
		Payload:    append([]byte(nil), data[8:]...),
	}, nil
}
