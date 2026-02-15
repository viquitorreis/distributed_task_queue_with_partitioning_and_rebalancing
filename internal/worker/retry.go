package worker

import (
	"context"
	"crypto/sha256"
	"dtq/internal/conn"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RetryScheduler struct {
	Worker     IWorker
	Conn       conn.IConn
	BaseDelay  time.Duration
	MaxRetries uint8
}

type IRetryBackoff interface {
	ScheduleRetryTask(task ITask)
	ProcessRetries(ctx context.Context)
}

func NewRetryBackoff(w IWorker, maxRetries uint8, baseDelay time.Duration) IRetryBackoff {
	rs := &RetryScheduler{
		Worker:     w,
		Conn:       conn.NewConn(),
		BaseDelay:  baseDelay,
		MaxRetries: maxRetries,
	}

	go rs.GetTotalRetriedTasks()

	return rs
}

func (r *RetryScheduler) ScheduleRetryTask(task ITask) {
	if task.AttemptCount() <= r.MaxRetries {
		rt := &RetryTask{
			Payload:       task.ReadTask(),
			AttCount:      task.AttemptCount() + 1,
			NextRetryTime: time.Now().Add(r.BaseDelay * time.Duration(1<<task.AttemptCount())), // 2^n
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

		r.Worker.GetMetrics().IncrRetried()
	} else {
		slog.Warn("sending to DLQ")
		r.Conn.GetRedis().LPush(
			context.Background(),
			"dlq:tasks",
			task.ReadTask(),
		)

		r.Worker.GetMetrics().IncrDead()

		// expire dlq after 7 days
		r.Conn.GetRedis().Expire(
			context.Background(),
			"dlq:tasks",
			7*24*time.Hour,
		)
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

func CalcTaskRetryPartition(task string) byte {
	hash := sha256.Sum256([]byte(task))
	return hash[rand.IntN(32)]
}

func (r *RetryScheduler) GetTotalRetriedTasks() int64 {
	var retried int64

	for _, partitions := range r.Worker.GetOwnedRetryPartitions() {
		res, err := r.Conn.GetRedis().ZCount(
			context.Background(),
			fmt.Sprintf("retries:%d", partitions),
			"0",
			"+inf",
		).Result()
		if err != nil {
			slog.Warn("error getting retried tasks from redis", "error", err)
			continue
		}
		retried += res
	}

	r.Worker.GetMetrics().SetTotalRetriedTasks(retried)

	return retried
}
