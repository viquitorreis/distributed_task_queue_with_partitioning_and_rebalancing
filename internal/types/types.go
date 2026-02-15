package types

import (
	"os"
	"strconv"
	"time"
)

var NODE_ID = ""

type WorkerID string

const NUM_VNODES = 120

func GetRingPatitions() int {
	val := os.Getenv("RING_PARTITIONS")
	if val == "" {
		return 256
	}
	partitions, _ := strconv.Atoi(val)
	return partitions
}

func GetMaxRetries() uint8 {
	val := os.Getenv("MAX_RETRIES")
	if val == "" {
		return 10
	}
	retries, _ := strconv.Atoi(val)
	return uint8(retries)
}

func GetBaseRetryDelay() time.Duration {
	val := os.Getenv("BASE_RETRY_DELAY")
	if val == "" {
		return 10
	}
	baseRetry, _ := strconv.Atoi(val)
	return time.Second * time.Duration(baseRetry)
}
