package quictransport

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWriteReadFrame(t *testing.T) {
	t.Parallel()

	payload := []byte("raw-ipv4-packet")
	var buf bytes.Buffer

	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadFrame() = %q, want %q", got, payload)
	}
}

func TestFrameRejectsZeroLength(t *testing.T) {
	t.Parallel()

	if err := WriteFrame(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("WriteFrame(nil) error = nil, want error")
	}

	_, err := ReadFrame(bytes.NewReader([]byte{0, 0, 0, 0}))
	if err == nil {
		t.Fatal("ReadFrame(zero length) error = nil, want error")
	}
}

func TestFrameRejectsTooLarge(t *testing.T) {
	t.Parallel()

	tooLarge := make([]byte, MaxFrameSize+1)
	if err := WriteFrame(&bytes.Buffer{}, tooLarge); err == nil {
		t.Fatal("WriteFrame(too large) error = nil, want error")
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxFrameSize+1)
	_, err := ReadFrame(bytes.NewReader(header[:]))
	if err == nil {
		t.Fatal("ReadFrame(too large) error = nil, want error")
	}
}

func TestReadFrameRejectsIncompleteFrame(t *testing.T) {
	t.Parallel()

	if _, err := ReadFrame(bytes.NewReader([]byte{0, 0})); err == nil {
		t.Fatal("ReadFrame(incomplete header) error = nil, want error")
	}

	var buf bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 4)
	buf.Write(header[:])
	buf.Write([]byte{1, 2})

	if _, err := ReadFrame(&buf); err == nil {
		t.Fatal("ReadFrame(incomplete payload) error = nil, want error")
	}
}
