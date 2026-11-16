package app

import (
	"context"
	"encoding/json"
	"time"
)

// Jobs is the background job queue (PLT-09), implemented by platform/jobs on
// River. Kinds are "<module>.<name>", e.g. "notify.prune".
//
// Handle and Every only record what to run, so they belong in Init (no I/O).
// The queue starts after every module has started; a job whose kind has no
// handler on this server is cancelled instead of staying in the queue.
type Jobs interface {
	// Handle registers the worker for kind. A handler that returns an error
	// or panics is retried with growing delays, up to JobOptions.MaxAttempts.
	// Handlers must be idempotent: a job can run more than once.
	Handle(kind string, h JobHandler, opts JobOptions)
	// Every enqueues kind with args every interval, on one instance only.
	// kind must also have a Handle.
	Every(interval time.Duration, kind string, args any)
	// Enqueue inserts a job with args encoded as JSON. Inside TxManager.Do it
	// joins that transaction, so the job exists only if the data commits.
	Enqueue(ctx context.Context, kind string, args any, opts EnqueueOptions) error
}

// JobHandler runs one job. args is the JSON given to Enqueue or Every.
type JobHandler func(ctx context.Context, args json.RawMessage) error

type JobOptions struct {
	MaxAttempts int           // 0 means 5 (NOTI-08)
	Timeout     time.Duration // 0 means 1 minute
}

type EnqueueOptions struct {
	RunAt time.Time // zero means now
}
