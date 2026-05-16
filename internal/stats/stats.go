package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// Counters tracks packet flow through one component.
type Counters struct {
	rxPackets      atomic.Uint64
	rxBytes        atomic.Uint64
	txPackets      atomic.Uint64
	txBytes        atomic.Uint64
	droppedPackets atomic.Uint64
	errors         atomic.Uint64
}

// Snapshot is a stable point-in-time copy of Counters.
type Snapshot struct {
	RXPackets      uint64 `json:"rx_packets"`
	RXBytes        uint64 `json:"rx_bytes"`
	TXPackets      uint64 `json:"tx_packets"`
	TXBytes        uint64 `json:"tx_bytes"`
	DroppedPackets uint64 `json:"dropped_packets"`
	Errors         uint64 `json:"errors"`
}

// SnapshotFormatter renders a snapshot for logs.
type SnapshotFormatter func(Snapshot) string

// AddRX records one received packet.
func (c *Counters) AddRX(packetBytes int) {
	c.addPacket(&c.rxPackets, &c.rxBytes, packetBytes)
}

// AddTX records one transmitted packet.
func (c *Counters) AddTX(packetBytes int) {
	c.addPacket(&c.txPackets, &c.txBytes, packetBytes)
}

// AddDrop records one packet dropped before forwarding.
func (c *Counters) AddDrop() {
	c.droppedPackets.Add(1)
}

// AddError records one runtime error.
func (c *Counters) AddError() {
	c.errors.Add(1)
}

// Snapshot returns current counter values.
func (c *Counters) Snapshot() Snapshot {
	return Snapshot{
		RXPackets:      c.rxPackets.Load(),
		RXBytes:        c.rxBytes.Load(),
		TXPackets:      c.txPackets.Load(),
		TXBytes:        c.txBytes.Load(),
		DroppedPackets: c.droppedPackets.Load(),
		Errors:         c.errors.Load(),
	}
}

func (c *Counters) addPacket(packets *atomic.Uint64, bytes *atomic.Uint64, packetBytes int) {
	packets.Add(1)
	if packetBytes > 0 {
		bytes.Add(uint64(packetBytes))
	}
}

// String returns a compact log-friendly rendering.
func (s Snapshot) String() string {
	return fmt.Sprintf("rx=%d packets/%d bytes tx=%d packets/%d bytes dropped=%d errors=%d", s.RXPackets, s.RXBytes, s.TXPackets, s.TXBytes, s.DroppedPackets, s.Errors)
}

// TextSnapshot returns the default human-readable stats rendering.
func TextSnapshot(snapshot Snapshot) string {
	return snapshot.String()
}

// JSONSnapshot returns a stable machine-readable stats rendering.
func JSONSnapshot(snapshot Snapshot) string {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// LogEvery writes periodic snapshots until ctx is canceled.
func LogEvery(ctx context.Context, interval time.Duration, counters *Counters, logf func(string, ...any), formatters ...SnapshotFormatter) {
	if interval <= 0 || counters == nil || logf == nil {
		return
	}
	formatter := TextSnapshot
	if len(formatters) > 0 && formatters[0] != nil {
		formatter = formatters[0]
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logf("stats: %s", formatter(counters.Snapshot()))
		}
	}
}
