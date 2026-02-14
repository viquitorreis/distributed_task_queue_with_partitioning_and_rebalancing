package metrics

import (
	"dtq/internal/conn"
	"dtq/internal/observability"
	"dtq/internal/types"
	"log/slog"
	"sync"
	"time"
)

type Metrics struct {
	ProcessedTasks    uint64
	RebalancingCount  uint64
	TotalPartitions   uint64
	TotalTasksFailed  uint64
	TotalTasksRetried uint64
	TotalTasksOnDLQ   uint64
	RetryQueueDepth   uint64

	conn        conn.IConn
	WorkerID    types.WorkerID
	LogInterval time.Duration

	mu sync.RWMutex
}

type IMetrics interface {
	IncrTask()
	IncrRebalancing()
	IncrFailed()
	IncrRetried()
	IncrDead()
	SetTotalRetriedTasks(amount int64)
	SetPartitions(amount uint64)
	SetWorkerID(id types.WorkerID)
	DoMonitor()
}

func NewMetrics() IMetrics {
	m := &Metrics{
		conn:             conn.NewConn(),
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

func (m *Metrics) IncrFailed() {
	m.mu.Lock()
	m.TotalTasksFailed++
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.TasksFailedTotal.WithLabelValues(workerID).Inc()
}

func (m *Metrics) IncrRetried() {
	m.mu.Lock()
	m.TotalTasksRetried++
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.TasksRetriedTotal.WithLabelValues(workerID).Inc()
}

func (m *Metrics) IncrDead() {
	m.mu.Lock()
	m.TotalTasksOnDLQ++
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.TasksDeadTotal.WithLabelValues(workerID).Inc()
}

func (m *Metrics) SetTotalRetriedTasks(amount int64) {
	m.mu.Lock()
	m.RetryQueueDepth = uint64(amount)
	workerID := string(m.WorkerID)
	m.mu.Unlock()

	observability.RetryQueueDepth.WithLabelValues(workerID).Set(float64(amount))
}

func (m *Metrics) DoMonitor() {
	ticker := time.NewTicker(m.LogInterval)
	for range ticker.C {
		m.mu.RLock()
		slog.Info("[METRICS]",
			"Processed Tasks", m.ProcessedTasks,
			"Rebalancing Count", m.RebalancingCount,
			"Total Partitions", m.TotalPartitions,
			"Total tasks failed", m.TotalTasksFailed,
			"Total tasks retried", m.TotalTasksRetried,
			"Total tasks on DLQ", m.TotalTasksOnDLQ,
			"Retry queue depth", m.RetryQueueDepth,
			"WorkerID", m.WorkerID,
		)
		m.mu.RUnlock()
	}
}
