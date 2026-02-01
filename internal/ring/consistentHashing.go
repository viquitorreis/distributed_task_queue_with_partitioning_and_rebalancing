package ring

import (
	"dtq/internal/types"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/twmb/murmur3"
)

// VNode represents the VNode hash
type VNode uint32

type HashRing struct {
	Nodes            map[VNode]types.WorkerID
	VNodes           []VNode
	workerPartitions []uint8
	totalPartitions  int

	mu sync.RWMutex
}

type IHashRing interface {
	AddNodes(workerID types.WorkerID)
	GetNodeForPartition(partitionID uint8) types.WorkerID
	FetchPartitionsForNode(workerID types.WorkerID) []uint8
	GetNodePartitions(workerID types.WorkerID) []uint8
	RemoveNode(workerID types.WorkerID)
}

func NewConsistentHashRing(partitions int) IHashRing {
	return &HashRing{
		Nodes:           map[VNode]types.WorkerID{},
		VNodes:          make([]VNode, 0),
		totalPartitions: partitions,
	}
}

func hashFunc(key string) uint32 {
	return murmur3.Sum32([]byte(key))
}

func newVNodeKey(workerID types.WorkerID, i int) string {
	return fmt.Sprintf("%s-node-%d", workerID, i)
}

// a quantidade de VNodes NÃO GARANTE divisão exata. Eles vão melhorar a distribuição estatística,
// mas não controlar o número exato
// Consistent Hashing NÃO garante divisão perfeita, garante:
//  1. divisão razoavelmente uniforme (10%-20% variação)
//  2. movimento mínimo quando workers mudam
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

	fmt.Println("h.VNodes:", h.VNodes)
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

	// wraparound circular: não achou nada maior, volta ao inicio
	if idx >= len(h.VNodes) {
		idx = 0
	}

	// worker id do vnode encontrado
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

	h.workerPartitions = partitions

	return partitions
}

func (h *HashRing) GetNodePartitions(workerID types.WorkerID) []uint8 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.workerPartitions
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
