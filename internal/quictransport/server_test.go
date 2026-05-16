package quictransport

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"mvp-vpn-lite/internal/packet"
	"mvp-vpn-lite/internal/stats"
)

func TestServerTUNSessionWriteNextPathRoundRobin(t *testing.T) {
	t.Parallel()

	var counters stats.Counters
	session := newServerTUNSession(&scriptedDevice{}, &counters, packetPolicy{})
	stream0 := &bufferStream{}
	stream1 := &bufferStream{}
	session.addPath(0, stream0)
	session.addPath(1, stream1)

	if err := session.writeNextPath([]byte("packet-0")); err != nil {
		t.Fatalf("writeNextPath(packet-0) error = %v", err)
	}
	if err := session.writeNextPath([]byte("packet-1")); err != nil {
		t.Fatalf("writeNextPath(packet-1) error = %v", err)
	}

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
}

func TestServerTUNSessionForwardDevicePackets(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	device := &scriptedDevice{
		reads:  [][]byte{[]byte("reply")},
		cancel: cancel,
	}
	var counters stats.Counters
	session := newServerTUNSession(device, &counters, packetPolicy{})
	stream := &bufferStream{}
	session.addPath(0, stream)

	session.forwardDevicePackets(ctx, make(chan error, 1))

	got, err := ReadFrame(stream)
	if err != nil {
		t.Fatalf("ReadFrame(stream) error = %v", err)
	}
	if string(got) != "reply" {
		t.Fatalf("stream packet = %q, want reply", got)
	}
	if snapshot := counters.Snapshot(); snapshot.TXPackets != 1 || snapshot.TXBytes != 5 {
		t.Fatalf("TX stats = %s, want 1 packet/5 bytes", snapshot)
	}
}

func TestServerTUNSessionForwardDevicePacketsDropsPacketsDeniedByPolicy(t *testing.T) {
	t.Parallel()

	rawPacket, err := packet.BuildICMPEchoRequest(net.IPv4(192, 0, 2, 10), net.IPv4(10, 8, 0, 1), 1, 1, nil)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	policy, err := newPacketPolicy("10.8.0.0/24")
	if err != nil {
		t.Fatalf("newPacketPolicy() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	device := &scriptedDevice{
		reads:  [][]byte{rawPacket},
		cancel: cancel,
	}
	var counters stats.Counters
	session := newServerTUNSession(device, &counters, policy)
	stream := &bufferStream{}
	session.addPath(0, stream)
	errCh := make(chan error, 1)

	session.forwardDevicePackets(ctx, errCh)

	if stream.Len() != 0 {
		t.Fatalf("stream length = %d, want 0", stream.Len())
	}
	snapshot := counters.Snapshot()
	if snapshot.DroppedPackets != 1 || snapshot.TXPackets != 0 {
		t.Fatalf("stats = %s, want 1 drop and no tx", snapshot)
	}

	select {
	case err := <-errCh:
		t.Fatalf("forwardDevicePackets() unexpected error = %v", err)
	default:
	}
}

func TestServerTUNSessionForwardDevicePacketsRetriesNotPollableRead(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	device := &scriptedDevice{
		readErrors: []error{&os.PathError{Op: "read", Path: "/dev/net/tun", Err: errors.New("not pollable")}},
		reads:      [][]byte{[]byte("reply")},
		cancel:     cancel,
	}
	var counters stats.Counters
	session := newServerTUNSession(device, &counters, packetPolicy{})
	stream := &bufferStream{}
	session.addPath(0, stream)
	errCh := make(chan error, 1)

	session.forwardDevicePackets(ctx, errCh)

	got, err := ReadFrame(stream)
	if err != nil {
		t.Fatalf("ReadFrame(stream) error = %v", err)
	}
	if string(got) != "reply" {
		t.Fatalf("stream packet = %q, want reply", got)
	}
	snapshot := counters.Snapshot()
	if snapshot.Errors != 1 || snapshot.TXPackets != 1 || snapshot.TXBytes != 5 {
		t.Fatalf("stats = %s, want 1 transient error and 1 tx packet/5 bytes", snapshot)
	}

	select {
	case err := <-errCh:
		t.Fatalf("forwardDevicePackets() unexpected fatal error = %v", err)
	default:
	}
}
