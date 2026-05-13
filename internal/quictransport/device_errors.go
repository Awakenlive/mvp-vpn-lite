package quictransport

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

const (
	retryablePacketDeviceReadDelay  = 10 * time.Millisecond
	maxRetryablePacketDeviceReads   = 100
	notPollablePacketDeviceFragment = "not pollable"
)

func isClosedPacketDeviceError(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, os.ErrClosed)
}

func isRetryablePacketDeviceReadError(err error) bool {
	if err == nil {
		return false
	}

	// Go can surface runtime poll.ErrNotPollable from /dev/net/tun as text only.
	return strings.Contains(err.Error(), notPollablePacketDeviceFragment)
}

func shouldLogRetryablePacketDeviceRead(attempt int) bool {
	return attempt == 1 || attempt%10 == 0
}
