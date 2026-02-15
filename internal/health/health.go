package health

import (
	"dtq/internal/types"
	"encoding/json"
	"net/http"
	"time"
)

type HealthChecker struct {
	worker    IWorkerInfo
	startedAt time.Time
}

type IWorkerInfo interface {
	GetWorkerID() types.WorkerID
	GetOwnedPartitions() []uint8
	GetLeaseID() int64
}

type HealthResponse struct {
	WorkerID        string `json:"worker_id"`
	OwnedPartitions []int  `json:"owned_partitions"`
	LeaseID         int64  `json:"lease_id"`
	Uptime          string `json:"uptime"`
}

func NewHealthChecker(worker IWorkerInfo) *HealthChecker {
	return &HealthChecker{
		worker:    worker,
		startedAt: time.Now(),
	}
}

func (h *HealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		partitions := make([]int, len(h.worker.GetOwnedPartitions()))
		for i, p := range h.worker.GetOwnedPartitions() {
			partitions[i] = int(p)
		}

		resp := HealthResponse{
			WorkerID:        string(h.worker.GetWorkerID()),
			OwnedPartitions: partitions,
			LeaseID:         h.worker.GetLeaseID(),
			Uptime:          time.Since(h.startedAt).String(),
		}

		json.NewEncoder(w).Encode(resp)
	}
}
