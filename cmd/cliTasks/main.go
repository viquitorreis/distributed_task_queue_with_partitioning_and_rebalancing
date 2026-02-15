package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strconv"
	"time"

	"crypto/sha256"

	"github.com/redis/go-redis/v9"
)

func main() {
	slog.Info("task manager")

	tasks := 2000

	if len(os.Args) > 1 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil {
			slog.Error("invalid number of tasks, using default 2000", "error", err)
		} else {
			tasks = n
		}
	}

	fmt.Println("creating tasks: ", tasks)
	time.Sleep(time.Second * 5)

	CreateTask(tasks)
}

func getRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6543",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	return rdb
}

func CreateTask(tasks int) {

	ctx := context.Background()

	taskNames := []string{
		"process-image-1",
		"send-email-2",
		"generate-report-3",
		"calculate-stats-4",
		"cleanup-old-data-5",
	}

	for i := range tasks {
		taskName := taskNames[rand.IntN(len(taskNames))]
		taskID := fmt.Sprintf("%s-instance-%d", taskName, i)

		hash := sha256.Sum256([]byte(taskID))
		// first hash byte will be 0 - 255 -> 256 possible partitions
		partition := hash[0]

		queueName := fmt.Sprintf("tasks:%d", partition)

		err := getRedis().LPush(ctx, queueName, taskID).Err()
		if err != nil {
			fmt.Println("erro adicionando task:", err)
		}
	}

	fmt.Println("Tasks adicionadas com sucesso!")
}
