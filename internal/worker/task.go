package worker

import "time"

type ITask interface {
	ReadTask() string
	AttemptCount() uint8
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
