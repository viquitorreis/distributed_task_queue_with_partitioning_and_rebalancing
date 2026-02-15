package worker

import (
	"encoding/json"
	"log/slog"
	"time"
)

type ITask interface {
	ReadTask() string
	AttemptCount() uint8
}

type ITaskProcessor interface {
	Process(payload string) error
}

type FreshTask struct {
	Payload     string
	ProcessedAt time.Time
}

func (t FreshTask) ReadTask() string {
	return t.Payload
}

func (t FreshTask) AttemptCount() uint8 {
	return 0
}

type RetryTask struct {
	Payload       string
	AttCount      uint8
	NextRetryTime time.Time
}

func (t RetryTask) ReadTask() string {
	return t.Payload
}

func (t RetryTask) AttemptCount() uint8 {
	return t.AttCount
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
