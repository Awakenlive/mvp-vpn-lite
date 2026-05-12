package tun

import (
	"io"
	"strings"
)

const (
	// DefaultDeviceName is the device name used by the client setup scripts.
	DefaultDeviceName = "mvpvpn0"

	// DefaultServerDeviceName is the device name used by the server setup scripts.
	DefaultServerDeviceName = "mvpvpns0"

	// DefaultMTU leaves room for QUIC/UDP/IP overhead on common Ethernet links.
	DefaultMTU = 1400
)

// Device is an opened TUN interface. It reads and writes raw IPv4 packets.
type Device struct {
	file io.ReadWriteCloser
	name string
}

// Name returns the kernel interface name.
func (d *Device) Name() string {
	return d.name
}

// Read reads one packet from the TUN interface.
func (d *Device) Read(packet []byte) (int, error) {
	return d.file.Read(packet)
}

// Write writes one packet to the TUN interface.
func (d *Device) Write(packet []byte) (int, error) {
	return d.file.Write(packet)
}

// Close closes the TUN interface file descriptor.
func (d *Device) Close() error {
	return d.file.Close()
}

func normalizeDeviceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultDeviceName
	}
	return name
}
