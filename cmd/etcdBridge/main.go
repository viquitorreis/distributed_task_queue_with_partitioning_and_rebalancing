package etcdbridge

import (
	"context"
	"dtq/internal/types"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"

	etcd "go.etcd.io/etcd/client/v3"
)

type TargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

type Bridge struct {
	Etcd           *etcd.Client
	MemoryBridge   map[types.WorkerID]string
	TgroupFilePath string

	mu sync.RWMutex
}

type IEtcdBridge interface {
	WatchWorkers()
	LoadInitialWorkers()
	Persist()
}

func NewEtcdBridge(etcd *etcd.Client) IEtcdBridge {
	return &Bridge{
		Etcd:           etcd,
		MemoryBridge:   make(map[types.WorkerID]string),
		TgroupFilePath: "tgroups.json",
	}
}

func (b *Bridge) WatchWorkers() {
	ctx := context.Background()

	watchCh := b.Etcd.Watch(ctx, "worker_metrics:", etcd.WithPrefix())

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
							endpoint := string(event.Kv.Value)

							slog.Info("[ETCD-BRIDGE] 🟢 Worker joined", "id", workerID, "lease", event.Kv.Lease)

							b.mu.Lock()
							b.MemoryBridge[types.WorkerID(workerID)] = endpoint
							b.mu.Unlock()

							b.Persist()
						}
					case etcd.EventTypeDelete:
						// worker exited / lease expired
						parts := strings.Split(string(event.Kv.Key), ":")
						if len(parts) >= 2 {
							workerID := parts[1]

							slog.Info("[ETCD-BRIDGE] 🔴 Worker left", "id", workerID)

							b.mu.Lock()
							delete(b.MemoryBridge, types.WorkerID(workerID))
							b.mu.Unlock()

							b.Persist()
						}
					}
				}
			case <-ctx.Done():
				slog.Debug("ctx closed")
			}
		}
	}()
}

func (b *Bridge) LoadInitialWorkers() {
	resp, err := b.Etcd.Get(context.Background(), "worker_metrics:", etcd.WithPrefix())
	if err != nil {
		slog.Error("failed to load initial workers", "error", err)
		return
	}

	b.mu.Lock()
	for _, kv := range resp.Kvs {
		parts := strings.Split(string(kv.Key), ":")
		if len(parts) >= 2 {
			workerID := types.WorkerID(parts[1])
			endpoint := string(kv.Value)
			b.MemoryBridge[workerID] = endpoint
			slog.Info("[ETCD-BRIDGE] Loaded existing worker", "id", workerID, "endpoint", endpoint)
		}
	}
	b.mu.Unlock()

	b.Persist()
}

func (b *Bridge) Persist() {
	b.mu.RLock()
	var endpoints []string
	for _, endpoint := range b.MemoryBridge {
		endpoints = append(endpoints, endpoint)
	}
	b.mu.RUnlock()

	targets := &TargetGroup{
		Targets: endpoints,
		Labels: map[string]string{
			"job": "dtq-workers",
		},
	}

	content, err := json.Marshal([]*TargetGroup{targets})
	if err != nil {
		slog.Error("error marshalling tgroups data", "err", err.Error())
		return
	}

	f, err := os.Create(b.TgroupFilePath)
	if err != nil {
		slog.Error("error opening tgroups json", "err", err.Error())
		return
	}
	defer f.Close()

	if _, err := f.Write(content); err != nil {
		slog.Error("error writing content to tgroups file", "err", err.Error())
	}
}
