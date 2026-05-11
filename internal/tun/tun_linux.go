//go:build linux

package tun

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const devicePath = "/dev/net/tun"

// Open opens or creates a Linux TUN interface with IFF_NO_PI enabled.
func Open(name string) (*Device, error) {
	deviceName := normalizeDeviceName(name)
	ifreq, err := unix.NewIfreq(deviceName)
	if err != nil {
		return nil, fmt.Errorf("invalid TUN device name %q: %w", deviceName, err)
	}
	ifreq.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)

	file, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", devicePath, err)
	}
	if err := unix.IoctlIfreq(int(file.Fd()), unix.TUNSETIFF, ifreq); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("configure TUN %s: %w", deviceName, err)
	}

	return &Device{
		file: file,
		name: ifreq.Name(),
	}, nil
}
