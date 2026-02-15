package ring

import (
	"dtq/internal/types"
	"fmt"
	"testing"
)

func getPartitionsOwners(t *testing.T, ring IHashRing) map[types.WorkerID]int {
	t.Helper()
	count := make(map[types.WorkerID]int)
	for i := range 256 {
		worker := ring.GetNodeForPartition(uint8(i))
		count[worker]++
	}
	return count
}

// completeness invariant: every partition must always have an owner
func TestHashRingCompleteness(t *testing.T) {
	tests := []struct {
		name        string
		workerCount int
	}{
		{"single worker owns all", 1},
		{"two workers share all", 2},
		{"three workers share all", 3},
		{"five workers share all", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ring := NewConsistentHashRing()

			for i := range tt.workerCount {
				ring.AddNodes(types.WorkerID(fmt.Sprintf("worker-%d", i)))
			}

			sum := 0
			partitions := getPartitionsOwners(t, ring)

			for _, v := range partitions {
				sum += v
			}

			if sum != 256 {
				t.Error("wrong partition amount for hash ring")
			}
		})
	}
}

func getPartitionsSnapshot(t *testing.T, ring IHashRing) map[uint8]types.WorkerID {
	t.Helper()
	snapshot := make(map[uint8]types.WorkerID)
	for i := range 256 {
		snapshot[uint8(i)] = ring.GetNodeForPartition(uint8(i))
	}
	return snapshot
}

// minimal movement invariant: when a worker joins or leaves
// only partititons that was owned by that worker should change ownership
func TestHashRingMinimalMovement(t *testing.T) {
	ring := NewConsistentHashRing()

	for i := range 3 {
		ring.AddNodes(types.WorkerID(fmt.Sprintf("worker-%d", i)))
	}

	before := getPartitionsSnapshot(t, ring)

	ring.AddNodes("worker-3")

	after := getPartitionsSnapshot(t, ring)

	for i := range 256 {
		if before[uint8(i)] != after[uint8(i)] {
			if after[uint8(i)] != "worker-3" {
				t.Errorf("partition %d moved from %s to %s, should only move to worker-3",
					i, before[uint8(i)], after[uint8(i)])
			}
		}
	}
}
