package types

import "time"

var NODE_ID = ""

type WorkerID string

const NUM_VNODES = 120

const BASE_RETRY_DELAY = 10 * time.Second
