package metrics

import (
	"log/slog"
	"sync"
	"time"
)

type Metrics struct {
	ProcessedTasks   uint64
	RebalancingCount uint64
	TotalPartitions  uint64
	LogInterval      time.Duration

	mu sync.RWMutex
}

type IMetrics interface {
	IncrTask()
	IncrRebalancing()
	SetPartitions(amount uint64)
	DoMonitor()
}

func NewMetrics() IMetrics {
	m := &Metrics{
		ProcessedTasks:   0,
		RebalancingCount: 0,
		TotalPartitions:  0,
		LogInterval:      time.Second * 5,
	}

	go func() {
		m.DoMonitor()
	}()

	return m
}

func (m *Metrics) IncrTask() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProcessedTasks++
}

func (m *Metrics) IncrRebalancing() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RebalancingCount++
}

func (m *Metrics) SetPartitions(amount uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalPartitions = amount
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
