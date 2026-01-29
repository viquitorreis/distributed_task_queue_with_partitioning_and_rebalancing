package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	worker := NewWorker()
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
