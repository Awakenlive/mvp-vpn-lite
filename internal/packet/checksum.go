package packet

// Checksum returns the standard Internet checksum used by IPv4 and ICMP.
func Checksum(data []byte) uint16 {
	var sum uint32

	for len(data) >= 2 {
		sum += uint32(data[0])<<8 | uint32(data[1])
		data = data[2:]
	}

	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}

	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return ^uint16(sum)
}
