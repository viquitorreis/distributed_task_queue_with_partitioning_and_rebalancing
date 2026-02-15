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
	"time"
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

	port := worker.GetServerPort()

	fmt.Println("port: ", port)
	time.Sleep(time.Second * 5)
	srv := server.NewHTTPServer(port)
	srv.RegisterRoutes("/metrics", observability.MetricsHandler(prom))
	srv.RegisterRoutes("/health", health.NewHealthChecker(worker).Handler())
	go srv.Start()

	<-ctx.Done()
}
