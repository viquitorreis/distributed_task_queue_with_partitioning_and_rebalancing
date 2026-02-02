package worker

import (
	"context"
	"dtq/internal/conn"
	"dtq/internal/ring"
	"dtq/internal/types"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	etcd "go.etcd.io/etcd/client/v3"
)

type Worker struct {
	workerID   types.WorkerID
	leaseID    int64
	updateChan chan struct{}
	conn       conn.IConn
	Chr        ring.IHashRing

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

type IWorker interface {
	RunTask()
	CreateWorker()
	CreateLease()
	GetWorkers() []*Worker
	WatchWorkers()
	Shutdown()
}

func NewWorker(conn conn.IConn, chr ring.IHashRing) IWorker {
	// context with cancel because needs to be canceled when we need to rebalance
	ctx, cancel := context.WithCancel(context.Background())

	w := Worker{
		ctx:        ctx,
		cancel:     cancel,
		conn:       conn,
		Chr:        chr,
		updateChan: make(chan struct{}, 1),
	}

	w.CreateWorker()
	slog.Info("Worker up and running 👽", "id", w.workerID)

	// goroutine to detect rebalancing (updated workers on etcd)
	go func() {
		for range w.updateChan {
			slog.Info("Rebalancing detected, canceling current BLPOP")
			w.cancel()
		}
	}()

	go func() {
		for {
			time.Sleep(time.Second * 1)
			w.RunTask()
		}
	}()

	return &w
}

func (w *Worker) CreateWorker() {
	w.mu.Lock()
	defer w.mu.Unlock()

	host, err := os.Hostname()
	if err != nil {
		log.Fatalf("error getting worker hostname: %s", err)
	}
	timestamp := time.Now().UnixNano()
	w.workerID = types.WorkerID(fmt.Sprintf("worker-%s-%d-%d", host, timestamp, os.Getpid()))

	fmt.Println("worker id: ", w.workerID)

	fmt.Println("Conectando ao etcd...")
	w.CreateLease()
	fmt.Println("Lease criado")

	w.Chr.AddNodes(w.workerID)
	fmt.Println("Adicionado ao ring")

	myPartitions := w.Chr.FetchPartitionsForNode(w.workerID)
	fmt.Printf("Worker responsavel por %d partitions: %v\n", len(myPartitions), myPartitions)

	// w.workerPartitions = myPartitions

	workers := w.GetWorkers()

	fmt.Println("workers are: ", workers)

	w.WatchWorkers()
}

func (w *Worker) RunTask() {
	// blocking left pop
	// 1. Remove a task da lista
	// 2. é atomico
	// 3. bloqueia esperando se a lista esta vazia ao invez de retornar na hora
	if len(w.Chr.GetNodePartitions(w.workerID)) == 0 {
		time.Sleep(time.Second)
		return
	}

	partitions := make([]string, len(w.Chr.GetNodePartitions(w.workerID)))
	for i, partitionID := range w.Chr.GetNodePartitions(w.workerID) {
		partitions[i] = fmt.Sprintf("tasks:%d", partitionID)
	}

	res, err := w.conn.GetRedis().BLPop(w.ctx, 0, partitions...).Result()
	if err != nil {
		if err == context.Canceled {
			slog.Info("current blpop canceled. Workers udpated. Recreating context for new partitions...")
			// context cancelado, no proximo loop na main será recalculado suas partitions e chamar runTask novamente
			w.mu.Lock()
			w.ctx, w.cancel = context.WithCancel(context.Background())
			w.mu.Unlock()
			return
		}
		fmt.Println("err reading: ", err)
		return
	}

	slog.Info("worker processed task", "worker", w.workerID, "task", res[1], "partition", res[0])
}

func (w *Worker) CreateLease() {
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

	workerID := fmt.Sprintf("worker_id:%s", w.workerID)

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

func (w *Worker) GetWorkers() []*Worker {
	resp, err := w.conn.GetEtcd().Get(context.Background(), "worker_id", etcd.WithPrefix())
	if err != nil {
		log.Fatalf("err fetching workers from etcd: %v", err)
	}

	workers := make([]*Worker, 0)

	for _, kv := range resp.Kvs {
		parts := strings.Split(string(kv.Key), ":")
		if len(parts) < 2 {
			continue
		}
		workerID := parts[1]

		workers = append(workers, &Worker{
			workerID: types.WorkerID(workerID),
			leaseID:  kv.Lease,
		})

		fmt.Printf("Worker ID: %s, Lease ID: %d\n", workerID, kv.Lease)
	}

	return workers
}

func (w *Worker) WatchWorkers() {
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
							w.Chr.AddNodes(types.WorkerID(workerID))

							myPartitions := w.Chr.FetchPartitionsForNode(w.workerID)

							w.updateChan <- struct{}{}
							slog.Warn("worker agora é dono das partitions", "partitions", myPartitions)
						}
					case etcd.EventTypeDelete:
						// worker exited / lease expired
						parts := strings.Split(string(event.Kv.Key), ":")
						if len(parts) >= 2 {
							workerID := parts[1]
							slog.Info("🔴 Worker left", "id", workerID)

							// ------- recalcular partitions aqui com consistent hashing
							w.Chr.RemoveNode(types.WorkerID(workerID))

							myPartitions := w.Chr.FetchPartitionsForNode(w.workerID)
							slog.Warn("worker agora é dono das partitions", "partitions", myPartitions)

							w.updateChan <- struct{}{}
						}
					}
				}
			case <-ctx.Done():
				fmt.Println("ctx closed")
			}
		}
	}()
}

func (w *Worker) Shutdown() {
	w.mu.Lock()
	w.conn.Close()
	w.mu.Unlock()
}
