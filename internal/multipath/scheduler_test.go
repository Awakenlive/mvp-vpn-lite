package multipath

import "testing"

func TestRoundRobinNext(t *testing.T) {
	t.Parallel()

	scheduler, err := NewRoundRobin(2)
	if err != nil {
		t.Fatalf("NewRoundRobin() error = %v", err)
	}

	want := []int{0, 1, 0, 1, 0}
	for i, expected := range want {
		if got := scheduler.Next(); got != expected {
			t.Fatalf("Next() call %d = %d, want %d", i+1, got, expected)
		}
	}
}

func TestNewRoundRobinRejectsEmptyPathSet(t *testing.T) {
	t.Parallel()

	if _, err := NewRoundRobin(0); err == nil {
		t.Fatal("NewRoundRobin(0) error = nil, want error")
	}
}
