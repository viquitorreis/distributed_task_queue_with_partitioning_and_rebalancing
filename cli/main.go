package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"

	"crypto/sha256"

	"github.com/redis/go-redis/v9"
)

func main() {
	fmt.Println("task manager")
	createTask()
}

func getRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6543",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	// ctx := context.Background()
	return rdb
}

func createTask() {

	ctx := context.Background()

	taskNames := []string{
		"process-image-1",
		"send-email-2",
		"generate-report-3",
		"calculate-stats-4",
		"cleanup-old-data-5",
	}

	for i := range 200 {
		taskName := taskNames[rand.IntN(len(taskNames))]
		taskID := fmt.Sprintf("%s-instance-%d", taskName, i) // id unico para a task...

		hash := sha256.Sum256([]byte(taskName))
		partition := int(binary.BigEndian.Uint64(hash[:])) % 256 // 256 partitions

		queueName := fmt.Sprintf("tasks:%d", partition)

		err := getRedis().LPush(ctx, queueName, taskID).Err()
		if err != nil {
			fmt.Println("erro adicionando task:", err)
		}
	}

	fmt.Println("Tasks adicionadas com sucesso!")
}
