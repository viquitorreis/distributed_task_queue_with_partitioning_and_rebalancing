package main

import (
	"context"
	etcdbridge "dtq/cmd/etcdBridge"
	"dtq/internal/conn"
	"dtq/internal/health"
	"dtq/internal/metrics"
	"dtq/internal/observability"
	"dtq/internal/ring"
	"dtq/internal/server"
	"dtq/internal/worker"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ring := ring.NewConsistentHashRing()
	conn := conn.NewConn()
	metrics := metrics.NewMetrics()
	worker := worker.NewWorker(conn, ring, metrics, &worker.SimulatedProcessor{})
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

	port := worker.GetServerPort()
	srv := server.NewHTTPServer(port)
	srv.RegisterRoutes("/metrics", observability.MetricsHandler(prom))
	srv.RegisterRoutes("/health", health.NewHealthChecker(worker).Handler())
	go srv.Start()

	<-ctx.Done()
}
