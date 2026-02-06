package metrics

import (
	"dtq/internal/observability"
	"dtq/internal/types"
	"log/slog"
	"sync"
	"time"
)

type Metrics struct {
	ProcessedTasks   uint64
	RebalancingCount uint64
	TotalPartitions  uint64
	WorkerID         types.WorkerID
	LogInterval      time.Duration

	mu sync.RWMutex
}

type IMetrics interface {
	IncrTask()
	IncrRebalancing()
	SetPartitions(amount uint64)
	SetWorkerID(id types.WorkerID)
	DoMonitor()
}

func NewMetrics() IMetrics {
	m := &Metrics{
		ProcessedTasks:   0,
		RebalancingCount: 0,
		TotalPartitions:  0,
		LogInterval:      time.Second * 5,
	}

	go m.DoMonitor()

	return m
}

func (m *Metrics) SetWorkerID(id types.WorkerID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WorkerID = id
}

func (m *Metrics) IncrTask() {
	m.mu.Lock()
	m.ProcessedTasks++
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.TasksProcessedTotal.WithLabelValues(workerID).Inc()
}

func (m *Metrics) IncrRebalancing() {
	m.mu.Lock()
	m.RebalancingCount++
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.RebalancesTotal.WithLabelValues(workerID).Inc()
}

func (m *Metrics) SetPartitions(amount uint64) {
	m.mu.Lock()
	m.TotalPartitions = amount
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.PartitionsOwned.WithLabelValues(workerID).Set(float64(amount))
}

func (m *Metrics) DoMonitor() {
	ticker := time.NewTicker(m.LogInterval)
	for range ticker.C {
		m.mu.RLock()
		slog.Info("[METRICS]",
			"Processed Tasks", m.ProcessedTasks,
			"Rebalancing Count", m.RebalancingCount,
			"Total Partitions", m.TotalPartitions,
		)
		m.mu.RUnlock()
	}
}
