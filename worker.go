package main

import (
	"context"
	"dtq/conn"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"

	etcd "go.etcd.io/etcd/client/v3"
)

var NODE_ID = ""

type worker struct {
	id      string
	leaseID int64

	conn conn.IConn

	mu sync.Mutex
}

type IWorker interface {
	RunTask()
	CreateWorker()
	CreateLease()
	GetWorkers() []*worker
	WatchWorkers()
	Shutdown()
}

func NewWorker() IWorker {
	w := worker{
		conn: conn.NewConn(),
	}
	w.CreateWorker()
	slog.Info("Worker up and running 👽", "id", w.id)
	return &w
}

func (w *worker) CreateWorker() {
	w.mu.Lock()
	host, err := os.Hostname()
	if err != nil {
		log.Fatalf("error getting worker: %s", err)
	}

	w.id = fmt.Sprintf("node-%s-%s-%d", NODE_ID, host, os.Getpid())

	fmt.Println("worker id: ", w.id)

	w.CreateLease()

	workers := w.GetWorkers()

	fmt.Println("workers are: ", workers)

	w.WatchWorkers()

	w.mu.Unlock()
}

func (w *worker) RunTask() {
	// blocking left pop
	// 1. Remove a task da lista
	// 2. é atomico
	// 3. bloqueia esperando se a lista esta vazia ao invez de retornar na hora
	partitions := make([]string, 256)
	for i := range 256 {
		partitions[i] = fmt.Sprintf("tasks:%d", i)
	}

	res, err := w.conn.GetRedis().BLPop(context.Background(), 0, partitions...).Result()
	if err != nil {
		fmt.Println("err reading: ", err)
	}

	fmt.Println("res: ", res)
}

func (w *worker) CreateLease() {
	etcdCli := w.conn.GetEtcd()

	ctx := context.Background()

	l := etcd.NewLease(etcdCli)

	leaseResp, err := l.Grant(ctx, 10)
	if err != nil {
		fmt.Println("error issuing lease: ", leaseResp)
		return
	}

	w.leaseID = int64(leaseResp.ID)

	keepAliveChan, err := etcdCli.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		log.Fatalf("err trying to keep lease alive: %v", err)
	}

	go func() {
		for {
			select {
			case ka, ok := <-keepAliveChan:
				if !ok {
					fmt.Println("keep alive channel closed.")
					return
				}
				fmt.Printf("lease renewed succesfully. New TTL: %d\n", ka.TTL)
			case <-ctx.Done():
				fmt.Println("context canceled. Stopping monitor")
				return
			}
		}
	}()

	workerID := fmt.Sprintf("worker_id:%s", w.id)

	resp, err := etcdCli.Get(ctx, workerID)
	if err != nil {
		log.Fatalf("error getting etcd worker key: %v", err)
		return
	}

	if resp.Count == 0 {
		resp, err := etcdCli.Put(ctx, workerID, "live", etcd.WithLease(leaseResp.ID))
		if err != nil {
			log.Fatalf("error putting etcd worker key: %v", err)
			return
		}

		fmt.Println("resp:", resp)
	}

	fmt.Println("resp:", resp)
}

func (w *worker) GetWorkers() []*worker {
	resp, err := w.conn.GetEtcd().Get(context.Background(), "worker_id", etcd.WithPrefix())
	if err != nil {
		log.Fatalf("err fetching workers from etcd: %v", err)
	}

	workers := make([]*worker, 0)

	for _, kv := range resp.Kvs {
		parts := strings.Split(string(kv.Key), ":")
		if len(parts) < 2 {
			continue
		}
		workerID := parts[1]

		workers = append(workers, &worker{
			id:      workerID,
			leaseID: kv.Lease,
		})

		fmt.Printf("Worker ID: %s, Lease ID: %d\n", workerID, kv.Lease)
	}

	return workers
}

func (w *worker) WatchWorkers() {
	ctx := context.Background()

	watchCh := w.conn.GetEtcd().Watch(ctx, "worker_id:", etcd.WithPrefix())

	go func() {
		for {
			select {
			case watchResp := <-watchCh:
				for _, event := range watchResp.Events {
					switch event.Type {
					case etcd.EventTypePut:
						// new worker joined or updated
						parts := strings.Split(string(event.Kv.Key), ":")
						if len(parts) >= 2 {
							workerID := parts[1]
							slog.Info("🟢 Worker joined", "id", workerID, "lease", event.Kv.Lease)

							// ------- recalcular partitions aqui com consistent hashing
						}
					case etcd.EventTypeDelete:
						// worker exited / lease expired
						parts := strings.Split(string(event.Kv.Key), ":")
						if len(parts) >= 2 {
							workerID := parts[1]
							slog.Info("🔴 Worker left", "id", workerID)

							// ------- recalcular partitions aqui com consistent hashing
						}
					}
				}
			case <-ctx.Done():
				fmt.Println("ctx closed")
			}
		}
	}()
}

func (w *worker) Shutdown() {
	w.mu.Lock()
	w.conn.Close()
	w.mu.Unlock()
}
