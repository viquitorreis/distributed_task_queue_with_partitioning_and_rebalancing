package worker

import (
	"context"
	"dtq/internal/conn"
	"dtq/internal/metrics"
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
	workerID    types.WorkerID
	leaseID     int64
	metricsPort string
	updateChan  chan struct{}

	conn    conn.IConn
	chr     ring.IHashRing
	metrics metrics.IMetrics

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

type IWorker interface {
	RunTask()
	CreateWorker()
	CreateLease()
	GetWorkers() []*Worker
	GetWorkerID() types.WorkerID
	GetMetricsPort() string
	WatchWorkers()
	Shutdown()
}

func NewWorker(
	conn conn.IConn,
	chr ring.IHashRing,
	metrics metrics.IMetrics,
) IWorker {
	// context with cancel because needs to be canceled when we need to rebalance
	ctx, cancel := context.WithCancel(context.Background())

	w := Worker{
		ctx:        ctx,
		cancel:     cancel,
		conn:       conn,
		chr:        chr,
		metrics:    metrics,
		updateChan: make(chan struct{}, 1),
	}

	w.CreateWorker()
	metrics.SetWorkerID(w.workerID)

	slog.Info("Worker up and running 👽", "id", w.workerID)

	// goroutine to detect rebalancing (updated workers on etcd)
	go func() {
		for range w.updateChan {
			slog.Info("Rebalancing detected, canceling current BLPOP")
			w.UpdateMetrics()
			w.cancel()
		}
	}()

	go func() {
		for {
			// time.Sleep(time.Second * 1)
			w.RunTask()
		}
	}()

	return &w
}

func (w *Worker) UpdateMetrics() {
	w.mu.Lock()
	partitions := len(w.chr.GetNodePartitions(w.workerID))
	w.mu.Unlock()

	w.metrics.SetPartitions(uint64(partitions))
	w.metrics.IncrRebalancing()
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

	slog.Info("connecting to etcd...")

	w.CreateLease()
	w.CreateEtcdPrometheusDiscovery()

	slog.Info("lease created")

	w.chr.AddNodes(w.workerID)

	slog.Info("worker added to ring", "worker_id", w.workerID)

	myPartitions := w.chr.FetchPartitionsForNode(w.workerID)

	w.metrics.SetPartitions(uint64(len(myPartitions)))

	// workers := w.GetWorkers()

	w.WatchWorkers()
}

func (w *Worker) RunTask() {
	if len(w.chr.GetNodePartitions(w.workerID)) == 0 {
		time.Sleep(time.Second)
		return
	}

	partitions := make([]string, len(w.chr.GetNodePartitions(w.workerID)))
	for i, partitionID := range w.chr.GetNodePartitions(w.workerID) {
		partitions[i] = fmt.Sprintf("tasks:%d", partitionID)
	}

	res, err := w.conn.GetRedis().BLPop(w.ctx, 0, partitions...).Result()
	if err != nil {
		if err == context.Canceled {
			slog.Info("current blpop canceled. Workers udpated. Recreating context for new partitions...")
			// context canceled, on the next loop on our main func it will be recalculated its new partitions and call runTask again
			w.mu.Lock()
			w.ctx, w.cancel = context.WithCancel(context.Background())
			w.mu.Unlock()
			return
		}
		fmt.Println("err reading: ", err)
		return
	}

	w.metrics.IncrTask()

	slog.Info("worker processed task", "worker", w.workerID, "task", res[1], "partition", res[0])
}

func (w *Worker) CreateEtcdPrometheusDiscovery() {
	etcdCli := w.conn.GetEtcd()

	w.metricsPort = fmt.Sprintf("%d", 11111+(os.Getpid()%1000))

	key := fmt.Sprintf("worker_metrics:%s", w.workerID)
	endpoint := fmt.Sprintf("localhost:%s", w.metricsPort)
	_, err := etcdCli.Put(context.Background(), key, endpoint, etcd.WithLease(etcd.LeaseID(w.leaseID)))
	if err != nil {
		log.Fatalf("error putting etcd worker key: %v", err)
		return
	}
}

func (w *Worker) CreateLease() {
	etcdCli := w.conn.GetEtcd()

	ctx := context.Background()

	l := etcd.NewLease(etcdCli)

	leaseResp, err := l.Grant(ctx, 10)
	if err != nil {
		slog.Warn("error issuing lease", "error", leaseResp)
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
					slog.Info("keep alive channel closed")
					return
				}

				slog.Info("lease renewed succesfully", "New TTL", ka.TTL)

			case <-ctx.Done():
				slog.Info("context canceled. Stopping monitor")
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
		_, err := etcdCli.Put(ctx, workerID, "live", etcd.WithLease(leaseResp.ID))
		if err != nil {
			log.Fatalf("error putting etcd worker key: %v", err)
			return
		}

	}
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

		slog.Info("GetWorkers", "worker_id", workerID, "leaseID", kv.Lease)
	}

	return workers
}

func (w *Worker) GetWorkerID() types.WorkerID {
	return w.workerID
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
							w.chr.AddNodes(types.WorkerID(workerID))

							myPartitions := w.chr.FetchPartitionsForNode(w.workerID)

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
							w.chr.RemoveNode(types.WorkerID(workerID))

							myPartitions := w.chr.FetchPartitionsForNode(w.workerID)
							slog.Warn("worker agora é dono das partitions", "partitions", myPartitions)

							w.updateChan <- struct{}{}
						}
					}
				}
			case <-ctx.Done():
				slog.Debug("ctx closed")
			}
		}
	}()
}

func (w *Worker) GetMetricsPort() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.metricsPort
}

func (w *Worker) Shutdown() {
	slog.Info("shutting down worker gracefully...")

	// canceling any processment (stops blpop)
	w.cancel()

	if w.leaseID != 0 {
		slog.Info("revoking worker lease...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := w.conn.GetEtcd().Revoke(ctx, etcd.LeaseID(w.leaseID))
		if err != nil {
			slog.Warn("failed to revoke lease", "error", err)
		} else {
			slog.Info("lease revoked succesfully")
		}
	}

	w.mu.Lock()
	w.conn.Close()
	w.mu.Unlock()

	slog.Info("worker shudown complete")
}
