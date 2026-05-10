package quictransport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const MaxFrameSize = 65535

// WriteFrame writes one length-prefixed IPv4 packet frame.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 {
		return errors.New("frame payload is empty")
	}
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("frame payload too large: %d > %d", len(payload), MaxFrameSize)
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads one length-prefixed IPv4 packet frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header[:])
	if length == 0 {
		return nil, errors.New("frame payload is empty")
	}
	if length > MaxFrameSize {
		return nil, fmt.Errorf("frame payload too large: %d > %d", length, MaxFrameSize)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	return payload, nil
}
