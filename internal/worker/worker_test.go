package worker

import (
	"context"
	"crypto/sha256"
	"dtq/internal/conn"
	"dtq/internal/metrics"
	"dtq/internal/ring"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond * 500)
	}
	t.Fatalf("timeout waiting: %s", msg)
}

func TestWorkerRebalancing(t *testing.T) {
	ring := ring.NewConsistentHashRing()
	conn := conn.NewConn()

	worker1 := NewWorker(
		conn,
		ring,
		metrics.NewMetrics(),
		&SimulatedProcessor{},
	)
	worker2 := NewWorker(
		conn,
		ring,
		metrics.NewMetrics(),
		&SimulatedProcessor{},
	)

	waitFor(t, time.Second*10, func() bool {
		w1Partitions := len(worker1.GetOwnedPartitions())
		w2Partitions := len(worker2.GetOwnedPartitions())
		return w1Partitions < 256 && w2Partitions < 256 && w1Partitions+w2Partitions == 256
	}, "both workers should be registered and sharing partitions")

	cmd := exec.Command("make", "create-tasks", "N=500")
	cmd.Dir = "../../"
	if err := cmd.Run(); err != nil {
		t.Fatalf("error trying to create tasks: %v", err)
	}

	worker1.Shutdown()
	waitFor(t, time.Second*10, func() bool {
		return len(worker2.GetOwnedPartitions()) == 256
	}, "worker2 should own all 256 partitions after worker1 shutdown")
}

type AlwaysFailProcessor struct{}

func (p *AlwaysFailProcessor) Process(payload string) error {
	return fmt.Errorf("forced failure for testing: %s", payload)
}

func TestRetryFlowReachesDLQ(t *testing.T) {
	os.Setenv("MAX_RETRIES", "3")
	os.Setenv("BASE_RETRY_DELAY", "1")
	t.Cleanup(func() {
		os.Unsetenv("MAX_RETRIES")
		os.Unsetenv("BASE_RETRY_DELAY")
	})

	conn := conn.NewConn()

	worker := NewWorker(
		conn,
		ring.NewConsistentHashRing(),
		metrics.NewMetrics(),
		&AlwaysFailProcessor{},
	)

	payload := "process-image"
	hash := sha256.Sum256([]byte(payload))
	partition := hash[0]

	if err := conn.GetRedis().LPush(context.Background(), fmt.Sprintf("tasks:%d", partition), payload).Err(); err != nil {
		t.Errorf("error pushing task to redis: %v", err)
	}

	waitFor(t, time.Second*30, func() bool {
		res, err := conn.GetRedis().LRange(context.Background(), "dlq:tasks", 0, -1).Result()
		if err != nil {
			t.Errorf("error getting task from redis: %v", err)
		}

		for _, task := range res {
			if strings.EqualFold(task, payload) {
				return true
			}
		}

		return false
	}, "task never reached DLQ")

	worker.Shutdown()
}
