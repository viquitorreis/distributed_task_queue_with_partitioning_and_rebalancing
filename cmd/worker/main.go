package main

import (
	"context"
	etcdbridge "dtq/cmd/etcdBridge"
	"dtq/internal/conn"
	"dtq/internal/metrics"
	"dtq/internal/observability"
	"dtq/internal/ring"
	"dtq/internal/worker"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ring := ring.NewConsistentHashRing(256)
	conn := conn.NewConn()
	metrics := metrics.NewMetrics()
	worker := worker.NewWorker(conn, ring, metrics)
	prom := observability.InitPrometheus()
	etcdBridge := etcdbridge.NewEtcdBridge(conn.GetEtcd())
	etcdBridge.LoadInitialWorkers()
	etcdBridge.WatchWorkers()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-sigchan
		worker.Shutdown()
		cancel()
		fmt.Println("closing program...")
	}()

	go func() {
		port := worker.GetMetricsPort()
		observability.StartMetricsServer(prom, port)
	}()

	<-ctx.Done()
}
