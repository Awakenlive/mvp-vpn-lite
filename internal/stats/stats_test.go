package stats

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCountersSnapshot(t *testing.T) {
	t.Parallel()

	var counters Counters
	counters.AddRX(120)
	counters.AddTX(80)
	counters.AddTX(-1)
	counters.AddDrop()
	counters.AddError()

	snapshot := counters.Snapshot()
	if snapshot.RXPackets != 1 {
		t.Fatalf("RXPackets = %d, want 1", snapshot.RXPackets)
	}
	if snapshot.RXBytes != 120 {
		t.Fatalf("RXBytes = %d, want 120", snapshot.RXBytes)
	}
	if snapshot.TXPackets != 2 {
		t.Fatalf("TXPackets = %d, want 2", snapshot.TXPackets)
	}
	if snapshot.TXBytes != 80 {
		t.Fatalf("TXBytes = %d, want 80", snapshot.TXBytes)
	}
	if snapshot.DroppedPackets != 1 {
		t.Fatalf("DroppedPackets = %d, want 1", snapshot.DroppedPackets)
	}
	if snapshot.Errors != 1 {
		t.Fatalf("Errors = %d, want 1", snapshot.Errors)
	}
}

func TestSnapshotString(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{
		RXPackets:      1,
		RXBytes:        2,
		TXPackets:      3,
		TXBytes:        4,
		DroppedPackets: 5,
		Errors:         6,
	}

	got := snapshot.String()
	for _, want := range []string{"rx=1 packets/2 bytes", "tx=3 packets/4 bytes", "dropped=5", "errors=6"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Snapshot.String() = %q, missing %q", got, want)
		}
	}
}

func TestJSONSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{
		RXPackets:      1,
		RXBytes:        2,
		TXPackets:      3,
		TXBytes:        4,
		DroppedPackets: 5,
		Errors:         6,
	}

	got := JSONSnapshot(snapshot)
	var decoded map[string]uint64
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("JSONSnapshot() returned invalid JSON %q: %v", got, err)
	}
	for key, want := range map[string]uint64{
		"rx_packets":      1,
		"rx_bytes":        2,
		"tx_packets":      3,
		"tx_bytes":        4,
		"dropped_packets": 5,
		"errors":          6,
	} {
		if decoded[key] != want {
			t.Fatalf("JSONSnapshot()[%q] = %d, want %d", key, decoded[key], want)
		}
	}
}

func TestLogEveryHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var logged bool
	LogEvery(ctx, time.Millisecond, &Counters{}, func(string, ...any) {
		logged = true
	})
	if logged {
		t.Fatal("LogEvery() logged after context cancellation")
	}
}
