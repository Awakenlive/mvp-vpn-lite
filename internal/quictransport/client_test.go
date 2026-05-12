package quictransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"mvp-vpn-lite/internal/packet"
	"mvp-vpn-lite/internal/stats"

	"github.com/quic-go/quic-go"
)

func TestValidateEchoReply(t *testing.T) {
	t.Parallel()

	virtualIP := net.IPv4(10, 8, 0, 1)
	clientIP := net.IPv4(10, 8, 0, 2)
	payload := []byte("demo")

	request, err := packet.BuildICMPEchoRequest(clientIP, virtualIP, 0x4d56, 9, payload)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	reply, err := packet.BuildICMPEchoReply(request, virtualIP, clientIP)
	if err != nil {
		t.Fatalf("BuildICMPEchoReply() error = %v", err)
	}

	if err := validateEchoReply(reply, virtualIP, clientIP, 0x4d56, 9, payload); err != nil {
		t.Fatalf("validateEchoReply() error = %v", err)
	}
}

func TestValidateEchoReplyRejectsUnexpectedSequence(t *testing.T) {
	t.Parallel()

	virtualIP := net.IPv4(10, 8, 0, 1)
	clientIP := net.IPv4(10, 8, 0, 2)
	payload := []byte("demo")

	request, err := packet.BuildICMPEchoRequest(clientIP, virtualIP, 0x4d56, 9, payload)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	reply, err := packet.BuildICMPEchoReply(request, virtualIP, clientIP)
	if err != nil {
		t.Fatalf("BuildICMPEchoReply() error = %v", err)
	}

	if err := validateEchoReply(reply, virtualIP, clientIP, 0x4d56, 10, payload); err == nil {
		t.Fatal("validateEchoReply() error = nil, want error")
	}
}

func TestSendTUNPacketsUsesRoundRobinPaths(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	device := &scriptedDevice{
		reads:  [][]byte{[]byte("packet-0"), []byte("packet-1")},
		cancel: cancel,
	}
	stream0 := &bufferStream{}
	stream1 := &bufferStream{}
	pathSet := newClientPathSet([]clientPath{
		{id: 0, stream: stream0},
		{id: 1, stream: stream1},
	})
	var counters stats.Counters

	sendTUNPackets(ctx, device, pathSet, &counters, make(chan error, 1))

	got0, err := ReadFrame(stream0)
	if err != nil {
		t.Fatalf("ReadFrame(stream0) error = %v", err)
	}
	if string(got0) != "packet-0" {
		t.Fatalf("stream0 packet = %q, want packet-0", got0)
	}
	got1, err := ReadFrame(stream1)
	if err != nil {
		t.Fatalf("ReadFrame(stream1) error = %v", err)
	}
	if string(got1) != "packet-1" {
		t.Fatalf("stream1 packet = %q, want packet-1", got1)
	}
	if snapshot := counters.Snapshot(); snapshot.TXPackets != 2 || snapshot.TXBytes != 16 {
		t.Fatalf("TX stats = %s, want 2 packets/16 bytes", snapshot)
	}
}

func TestSendTUNPacketsSkipsFailedPath(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	device := &scriptedDevice{
		reads:  [][]byte{[]byte("packet")},
		cancel: cancel,
	}
	failed := &failingStream{err: errors.New("broken stream")}
	fallback := &bufferStream{}
	pathSet := newClientPathSet([]clientPath{
		{id: 0, stream: failed},
		{id: 1, stream: fallback},
	})
	var counters stats.Counters

	sendTUNPackets(ctx, device, pathSet, &counters, make(chan error, 1))

	got, err := ReadFrame(fallback)
	if err != nil {
		t.Fatalf("ReadFrame(fallback) error = %v", err)
	}
	if string(got) != "packet" {
		t.Fatalf("fallback packet = %q, want packet", got)
	}
	if !failed.closed {
		t.Fatal("failed path stream was not closed")
	}
	if active := pathSet.len(); active != 1 {
		t.Fatalf("active paths = %d, want 1", active)
	}
	snapshot := counters.Snapshot()
	if snapshot.TXPackets != 1 || snapshot.TXBytes != 6 || snapshot.Errors != 1 {
		t.Fatalf("stats = %s, want 1 tx packet/6 bytes and 1 error", snapshot)
	}
}

func TestSendTUNPacketsDropsWhenNoActivePath(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	device := &scriptedDevice{
		reads:  [][]byte{[]byte("packet")},
		cancel: cancel,
	}
	pathSet := newClientPathSet(nil)
	errCh := make(chan error, 1)
	var counters stats.Counters

	sendTUNPackets(ctx, device, pathSet, &counters, errCh)

	snapshot := counters.Snapshot()
	if snapshot.DroppedPackets != 1 {
		t.Fatalf("drops = %d, want 1", snapshot.DroppedPackets)
	}

	select {
	case err := <-errCh:
		t.Fatalf("sendTUNPackets() unexpected error = %v", err)
	default:
	}
}

func TestClientPathSetAddReplacesPath(t *testing.T) {
	t.Parallel()

	oldStream := &bufferStream{}
	newStream := &bufferStream{}
	pathSet := newClientPathSet([]clientPath{{id: 0, stream: oldStream}})

	oldPath, replaced := pathSet.add(clientPath{id: 0, stream: newStream})
	if !replaced {
		t.Fatal("add() replaced = false, want true")
	}
	if oldPath.stream != oldStream {
		t.Fatal("add() did not return the replaced path")
	}
	if active := pathSet.len(); active != 1 {
		t.Fatalf("active paths = %d, want 1", active)
	}

	path, err := pathSet.nextPath()
	if err != nil {
		t.Fatalf("nextPath() error = %v", err)
	}
	if path.stream != newStream {
		t.Fatal("nextPath() returned the old stream, want replacement")
	}
}

func TestReceivePathPacketsWritesToTUNDevice(t *testing.T) {
	t.Parallel()

	stream := &bufferStream{}
	if err := WriteFrame(stream, []byte("reply")); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	device := &scriptedDevice{}
	errCh := make(chan error, 1)
	var counters stats.Counters
	pathSet := newClientPathSet([]clientPath{{id: 0, stream: stream}})

	done := make(chan struct{})
	go func() {
		receivePathPackets(context.Background(), clientPath{id: 0, stream: stream}, device, &sync.Mutex{}, pathSet, &counters, errCh)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("receivePathPackets() did not return after graceful EOF")
	}

	writes := device.Writes()
	if len(writes) != 1 {
		t.Fatalf("device writes = %d, want 1", len(writes))
	}
	if string(writes[0]) != "reply" {
		t.Fatalf("device write = %q, want reply", writes[0])
	}
	if snapshot := counters.Snapshot(); snapshot.RXPackets != 1 || snapshot.RXBytes != 5 {
		t.Fatalf("RX stats = %s, want 1 packet/5 bytes", snapshot)
	}
	if active := pathSet.len(); active != 0 {
		t.Fatalf("active paths = %d, want 0", active)
	}

	select {
	case err := <-errCh:
		t.Fatalf("receivePathPackets() unexpected error = %v", err)
	default:
	}
}

func TestReceivePathPacketsRemovesFailedPathWithoutFatalError(t *testing.T) {
	t.Parallel()

	stream := &readErrorStream{err: errors.New("broken read")}
	errCh := make(chan error, 1)
	var counters stats.Counters
	pathSet := newClientPathSet([]clientPath{{id: 0, stream: stream}})

	receivePathPackets(context.Background(), clientPath{id: 0, stream: stream}, &scriptedDevice{}, &sync.Mutex{}, pathSet, &counters, errCh)

	if active := pathSet.len(); active != 0 {
		t.Fatalf("active paths = %d, want 0", active)
	}
	if !stream.closed {
		t.Fatal("failed path stream was not closed")
	}
	if snapshot := counters.Snapshot(); snapshot.Errors != 1 {
		t.Fatalf("errors = %d, want 1", snapshot.Errors)
	}

	select {
	case err := <-errCh:
		t.Fatalf("receivePathPackets() unexpected fatal error = %v", err)
	default:
	}
}

func TestReconnectBackoffClampsAndResets(t *testing.T) {
	t.Parallel()

	backoff := newReconnectBackoff(100*time.Millisecond, 250*time.Millisecond)
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		250 * time.Millisecond,
	}
	for i, expected := range want {
		if got := backoff.next(); got != expected {
			t.Fatalf("next(%d) = %s, want %s", i, got, expected)
		}
	}

	backoff.reset()
	if got := backoff.next(); got != 100*time.Millisecond {
		t.Fatalf("next() after reset = %s, want 100ms", got)
	}
}

func TestNormalizeTUNReconnectIntervals(t *testing.T) {
	t.Parallel()

	minDelay, maxDelay, err := normalizeTUNReconnectIntervals(0, 0)
	if err != nil {
		t.Fatalf("normalizeTUNReconnectIntervals(defaults) error = %v", err)
	}
	if minDelay != DefaultTUNReconnectMinInterval || maxDelay != DefaultTUNReconnectMaxInterval {
		t.Fatalf("defaults = %s/%s, want %s/%s", minDelay, maxDelay, DefaultTUNReconnectMinInterval, DefaultTUNReconnectMaxInterval)
	}

	if _, _, err := normalizeTUNReconnectIntervals(2*time.Second, time.Second); err == nil {
		t.Fatal("normalizeTUNReconnectIntervals(min > max) error = nil, want error")
	}
}

func TestIsGracefulStreamEnd(t *testing.T) {
	t.Parallel()

	if !isGracefulStreamEnd(io.EOF) {
		t.Fatal("isGracefulStreamEnd(io.EOF) = false, want true")
	}
	if !isGracefulStreamEnd(&quic.ApplicationError{Remote: true, ErrorCode: 0}) {
		t.Fatal("isGracefulStreamEnd(application close 0) = false, want true")
	}
	if isGracefulStreamEnd(errors.New("boom")) {
		t.Fatal("isGracefulStreamEnd(generic error) = true, want false")
	}
}

type scriptedDevice struct {
	mu     sync.Mutex
	reads  [][]byte
	writes [][]byte
	cancel context.CancelFunc
}

func (d *scriptedDevice) Read(packet []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.reads) == 0 {
		if d.cancel != nil {
			d.cancel()
		}
		return 0, os.ErrClosed
	}

	read := d.reads[0]
	d.reads = d.reads[1:]
	return copy(packet, read), nil
}

func (d *scriptedDevice) Write(packet []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.writes = append(d.writes, append([]byte(nil), packet...))
	return len(packet), nil
}

func (d *scriptedDevice) Close() error {
	return nil
}

func (d *scriptedDevice) Writes() [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()

	writes := make([][]byte, len(d.writes))
	for i := range d.writes {
		writes[i] = append([]byte(nil), d.writes[i]...)
	}
	return writes
}

type bufferStream struct {
	bytes.Buffer
}

func (s *bufferStream) Close() error {
	return nil
}

func (s *bufferStream) SetDeadline(time.Time) error {
	return nil
}

type failingStream struct {
	err    error
	closed bool
}

func (s *failingStream) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (s *failingStream) Write([]byte) (int, error) {
	return 0, s.err
}

func (s *failingStream) Close() error {
	s.closed = true
	return nil
}

func (s *failingStream) SetDeadline(time.Time) error {
	return nil
}

type readErrorStream struct {
	err    error
	closed bool
}

func (s *readErrorStream) Read([]byte) (int, error) {
	return 0, s.err
}

func (s *readErrorStream) Write(packet []byte) (int, error) {
	return len(packet), nil
}

func (s *readErrorStream) Close() error {
	s.closed = true
	return nil
}

func (s *readErrorStream) SetDeadline(time.Time) error {
	return nil
}
