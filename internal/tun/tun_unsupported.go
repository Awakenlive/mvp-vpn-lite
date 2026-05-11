//go:build !linux

package tun

import (
	"fmt"
	"runtime"
)

// Open reports that TUN mode is only implemented for Linux in this MVP.
func Open(name string) (*Device, error) {
	return nil, fmt.Errorf("TUN devices are not supported on %s", runtime.GOOS)
}
