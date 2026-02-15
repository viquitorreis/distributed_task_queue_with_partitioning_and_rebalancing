package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	TasksProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dtq_tasks_processed_total",
		Help: "Total tasks processed by worker",
	}, []string{"worker_id"})
	PartitionsOwned = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dtq_partitions_owned",
		Help: "Total partitions owned by worker",
	}, []string{"worker_id"})
	RebalancesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dtq_rebalances_total",
		Help: "Total consistent hashing rebalances",
	}, []string{"worker_id"})
	TasksFailedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dtq_tasks_failed_total",
		Help: "Total tasks failed",
	}, []string{"worker_id"})
	TasksRetriedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dtq_tasks_retried_total",
		Help: "Total tasks retried after failing at least once",
	}, []string{"worker_id"})
	TasksDeadTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dtq_tasks_dead_total",
		Help: "Total tasks sent to dead letter queue",
	}, []string{"worker_id"})
	RetryQueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dtq_retry_queue_depth",
		Help: "Total length of retry queue",
	}, []string{"worker_id"})
)

func InitPrometheus() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(TasksProcessedTotal)
	reg.MustRegister(PartitionsOwned)
	reg.MustRegister(RebalancesTotal)
	reg.MustRegister(TasksFailedTotal)
	reg.MustRegister(TasksRetriedTotal)
	reg.MustRegister(TasksDeadTotal)
	reg.MustRegister(RetryQueueDepth)
	return reg
}

func MetricsHandler(reg *prometheus.Registry) http.HandlerFunc {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP
}
