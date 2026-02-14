package worker

import (
	"context"
	"crypto/sha256"
	"dtq/internal/conn"
	"dtq/internal/types"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RetryTask struct {
	Payload       string
	AttCount      uint8
	NextRetryTime time.Time
}

type RetryScheduler struct {
	Worker     IWorker
	Conn       conn.IConn
	MaxRetries uint8
}

type IRetryBackoff interface {
	ScheduleRetryTask(task ITask)
	ProcessRetries(ctx context.Context)
}

func NewRetryBackoff(w IWorker) IRetryBackoff {
	return &RetryScheduler{
		Worker:     w,
		Conn:       conn.NewConn(),
		MaxRetries: 10,
	}
}

func (r *RetryScheduler) ScheduleRetryTask(task ITask) {
	if task.AttemptCount() <= r.MaxRetries {
		rt := &RetryTask{
			Payload:       task.ReadTask(),
			AttCount:      task.AttemptCount() + 1,
			NextRetryTime: time.Now().Add(types.BASE_RETRY_DELAY * time.Duration(1<<task.AttemptCount())), // 2^n
		}

		json, err := rt.ToJSON()
		if err != nil {
			slog.Error("error serializing retry task into json", "err", err)
			return
		}

		partition := CalcTaskRetryPartition(task.ReadTask())

		r.Conn.GetRedis().ZAdd(context.Background(), fmt.Sprintf("retries:%d", partition), redis.Z{
			Score:  float64(rt.NextRetryTime.Unix()),
			Member: json,
		})
	} else {
		slog.Warn("sending to DLQ")
	}
}

func (r *RetryScheduler) ProcessRetries(ctx context.Context) {
	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	for range ticker.C {
		retryPartitions := r.Worker.GetOwnedRetryPartitions()

		for _, partition := range retryPartitions {
			tasks, err := r.Conn.GetRedis().ZRangeByScore(
				ctx,
				fmt.Sprintf("retries:%d", partition),
				// range score is the Unix timestamp when the task is ready to retry
				&redis.ZRangeBy{
					Min: "0",
					Max: strconv.FormatInt(time.Now().Unix(), 10),
				},
			).Result()
			if err != nil {
				slog.Error("error getting retry tasks from redis", "error", err)
			}

			for _, retryTask := range tasks {
				task, _ := RetryFromJSON(retryTask)
				if err := r.Worker.RunTask(task); err != nil {
					slog.Error("error retrying task", "error", err)
					if task.AttCount < r.MaxRetries {
						r.Conn.GetRedis().ZRem(
							context.Background(),
							fmt.Sprintf("retries:%d", partition),
							retryTask,
						)

						r.ScheduleRetryTask(task)

						continue
					} else {
						// dlq
					}
				}

				r.Conn.GetRedis().ZRem(
					context.Background(),
					fmt.Sprintf("retries:%d", partition),
					retryTask,
				)
			}
		}
	}
}

func (r *RetryTask) ToJSON() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		slog.Error("error marshalling retry task payload", "err", err)
		return "", err
	}
	return string(b), nil
}

func RetryFromJSON(s string) (*RetryTask, error) {
	var rt RetryTask
	if err := json.Unmarshal([]byte(s), &rt); err != nil {
		slog.Error("error unmarshalling retry task", "err", err)
		return nil, err
	}
	return &rt, nil
}

func CalcTaskRetryPartition(task string) byte {
	hash := sha256.Sum256([]byte(task))
	return hash[rand.IntN(32)]
}

func (t RetryTask) ReadTask() string {
	return t.Payload
}

func (t RetryTask) AttemptCount() uint8 {
	return t.AttCount
}
