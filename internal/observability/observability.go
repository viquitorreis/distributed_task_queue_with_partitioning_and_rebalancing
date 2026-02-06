package observability

import (
	"log/slog"
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
)

func InitPrometheus() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(TasksProcessedTotal)
	reg.MustRegister(PartitionsOwned)
	reg.MustRegister(RebalancesTotal)
	return reg
}

func StartMetricsServer(reg *prometheus.Registry, port string) {
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	slog.Info("Starting metrics server", "port", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		slog.Error("Failed to start metrics server", "error", err, "port", port)
	}
}
