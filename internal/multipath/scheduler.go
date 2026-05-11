package multipath

import "fmt"

// RoundRobin returns path indexes in order and wraps after the last path.
type RoundRobin struct {
	pathCount int
	next      int
}

// NewRoundRobin creates a scheduler for path indexes [0, pathCount).
func NewRoundRobin(pathCount int) (*RoundRobin, error) {
	if pathCount <= 0 {
		return nil, fmt.Errorf("path count must be positive: %d", pathCount)
	}

	return &RoundRobin{pathCount: pathCount}, nil
}

// Next returns the next path index.
func (r *RoundRobin) Next() int {
	path := r.next
	r.next = (r.next + 1) % r.pathCount
	return path
}
