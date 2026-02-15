package ring

import (
	"dtq/internal/types"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/twmb/murmur3"
)

type VNode uint32

type HashRing struct {
	Nodes           map[VNode]types.WorkerID
	VNodes          []VNode
	totalPartitions int

	mu sync.RWMutex
}

type IHashRing interface {
	AddNodes(workerID types.WorkerID)
	GetNodeForPartition(partitionID uint8) types.WorkerID
	FetchPartitionsForNode(workerID types.WorkerID) []uint8
	GetNodeForRetryPartition(partitionID uint8) types.WorkerID
	FetchRetryPartitionsForNode(workerID types.WorkerID) []uint8
	RemoveNode(workerID types.WorkerID)
}

func NewConsistentHashRing() IHashRing {
	return &HashRing{
		Nodes:           map[VNode]types.WorkerID{},
		VNodes:          make([]VNode, 0),
		totalPartitions: types.GetRingPatitions(),
	}
}

func hashFunc(key string) uint32 {
	return murmur3.Sum32([]byte(key))
}

func newVNodeKey(workerID types.WorkerID, i int) string {
	return fmt.Sprintf("%s-node-%d", workerID, i)
}

// VNode quantity does not guarantee exact division. They will improve statistical distribution, but not control exact number.
// Consistent Hashing does NOT guarantee perfect division, it guarantees:
//  1. reasonably uniform division (10%-20% variation)
//  2. minimal movement when workers change
func (h *HashRing) AddNodes(workerID types.WorkerID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range types.NUM_VNODES {
		vnodeKey := newVNodeKey(workerID, int(i))
		hash := VNode(hashFunc(vnodeKey))

		h.Nodes[hash] = types.WorkerID(workerID)
		h.VNodes = append(h.VNodes, hash)
	}

	slices.Sort(h.VNodes)
}

func (h *HashRing) GetNodeForPartition(partitionID uint8) types.WorkerID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.VNodes) == 0 {
		return ""
	}

	partitionKey := fmt.Sprintf("partition:%d", partitionID)
	partitionHash := VNode(hashFunc(partitionKey))

	// search first hash >= partitionHash
	idx := sort.Search(len(h.VNodes), func(i int) bool {
		return h.VNodes[i] >= partitionHash
	})

	// circular wraparound
	if idx >= len(h.VNodes) {
		idx = 0
	}

	vnodeHash := h.VNodes[idx]
	return h.Nodes[vnodeHash]
}

func (h *HashRing) FetchPartitionsForNode(workerID types.WorkerID) []uint8 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.VNodes) == 0 {
		return []uint8{}
	}

	partitions := make([]uint8, 0)

	for i := 0; i < h.totalPartitions; i++ {
		partitionKey := fmt.Sprintf("partition:%d", i)
		partitionHash := VNode(hashFunc(partitionKey))

		idx := sort.Search(len(h.VNodes), func(i int) bool {
			return h.VNodes[i] >= partitionHash
		})

		if idx >= len(h.VNodes) {
			idx = 0
		}

		owner := h.Nodes[h.VNodes[idx]]
		if owner == workerID {
			partitions = append(partitions, uint8(i))
		}
	}

	return partitions
}

func (h *HashRing) GetNodeForRetryPartition(partitionID uint8) types.WorkerID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.VNodes) == 0 {
		return ""
	}

	partitionKey := fmt.Sprintf("retry_partition:%d", partitionID)
	partitionHash := VNode(hashFunc(partitionKey))

	// search first hash >= partitionHash
	idx := sort.Search(len(h.VNodes), func(i int) bool {
		return h.VNodes[i] >= partitionHash
	})

	// circular wraparound
	if idx >= len(h.VNodes) {
		idx = 0
	}

	vnodeHash := h.VNodes[idx]
	return h.Nodes[vnodeHash]
}

func (h *HashRing) FetchRetryPartitionsForNode(workerID types.WorkerID) []uint8 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.VNodes) == 0 {
		return []uint8{}
	}

	partitions := make([]uint8, 0)

	for i := 0; i < h.totalPartitions; i++ {
		partitionKey := fmt.Sprintf("retry_partition:%d", i)
		partitionHash := VNode(hashFunc(partitionKey))

		idx := sort.Search(len(h.VNodes), func(i int) bool {
			return h.VNodes[i] >= partitionHash
		})

		if idx >= len(h.VNodes) {
			idx = 0
		}

		owner := h.Nodes[h.VNodes[idx]]
		if owner == workerID {
			partitions = append(partitions, uint8(i))
		}
	}

	return partitions
}

func (h *HashRing) RemoveNode(workerID types.WorkerID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range int(types.NUM_VNODES) {
		vnodeKey := newVNodeKey(workerID, i)
		hash := hashFunc(vnodeKey)
		delete(h.Nodes, VNode(hash))
	}

	// reconstruct vnodes after deleting from map
	h.VNodes = make([]VNode, 0, len(h.Nodes))
	for hash := range h.Nodes {
		h.VNodes = append(h.VNodes, hash)
	}

	slices.Sort(h.VNodes)
}
