package main

import (
	"context"
	"dtq/internal/conn"
	"dtq/internal/ring"
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
	worker := worker.NewWorker(conn, ring)

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-sigchan
		worker.Shutdown()
		cancel()
		fmt.Println("closing program...")
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			worker.RunTask()
			time.Sleep(time.Millisecond * 150)
		}
	}
}
