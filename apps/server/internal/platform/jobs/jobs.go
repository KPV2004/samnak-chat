package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
)

const (
	// defaultMaxAttempts is the retry limit from the plan (NOTI-08).
	defaultMaxAttempts = 5
	// defaultTimeout bounds one handler run; the plan sets a minute.
	defaultTimeout = time.Minute
	// defaultSweepInterval is how often unknown-kind jobs are cancelled.
	defaultSweepInterval = time.Minute
	// maxWorkers is River's concurrency per queue.
	maxWorkers = 10
	// sweepBatch bounds one cancellation query.
	sweepBatch = 1000
	// noopKind is reserved for the platform. River refuses to start a client
	// with no worker at all, and a server with no job handlers (notify
	// disabled) must still start so Enqueue and the sweeper work.
	noopKind = "platform.noop"
)

// queuedStates are the River states a job can be in while it waits for a
// worker. A job that has no worker on this server is cancelled from here.
var queuedStates = []string{"available", "scheduled", "retryable"}

// jobArgs is the one argument type every kind uses. River needs a Go type per
// worker; the handler is chosen from Kind at run time, so modules can add job
// kinds without a platform change.
type jobArgs struct {
	Name string          `json:"kind"`
	JSON json.RawMessage `json:"json"`
}

func (a jobArgs) Kind() string { return a.Name }

// registration is what Handle recorded in Init.
type registration struct {
	handler app.JobHandler
	opts    app.JobOptions
}

// period is what Every recorded in Init.
type period struct {
	interval time.Duration
	args     jobArgs
}

// Queue implements app.Jobs on River. New records nothing and does no I/O;
// Start builds and starts the River client, Stop drains it (NFR-11).
type Queue struct {
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu            sync.Mutex
	handlers      map[string]registration
	periods       []period
	started       bool
	client        *river.Client[pgx.Tx]
	stopSweep     context.CancelFunc
	sweepInterval time.Duration
	sweepWG       sync.WaitGroup
}

var _ app.Jobs = (*Queue)(nil)

// New returns a queue on pool. It performs no I/O; Handle and Every record
// work, Start starts it.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	return &Queue{
		pool:          pool,
		logger:        logger,
		handlers:      map[string]registration{},
		sweepInterval: defaultSweepInterval,
	}
}

// SetSweepInterval changes how often jobs without a worker are cancelled. It
// exists for tests; the default of one minute matches the plan. A call after
// Start is reported and ignored.
func (q *Queue) SetSweepInterval(d time.Duration) {
	if d <= 0 {
		q.logger.Error("jobs: SetSweepInterval needs a positive duration", "interval", d)
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		q.logger.Error("jobs: SetSweepInterval called after Start; ignored", "interval", d)
		return
	}
	q.sweepInterval = d
}

// Handle registers the worker for kind. It records the handler only; Start
// registers the River workers. A call after Start is reported and ignored.
func (q *Queue) Handle(kind string, h app.JobHandler, opts app.JobOptions) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		q.logger.ErrorContext(context.Background(), "jobs: Handle called after Start; ignored", "kind", kind)
		return
	}
	if kind == "" {
		q.logger.ErrorContext(context.Background(), "jobs: Handle needs a kind")
		return
	}
	if h == nil {
		q.logger.ErrorContext(context.Background(), "jobs: Handle needs a handler", "kind", kind)
		return
	}
	if _, dup := q.handlers[kind]; dup {
		q.logger.ErrorContext(context.Background(), "jobs: kind already has a handler", "kind", kind)
		return
	}
	q.handlers[kind] = registration{handler: h, opts: opts}
}

// Every enqueues kind with args every interval on the elected leader. It
// records the schedule only; Start installs River's periodic job. A call
// after Start, or a schedule without a handler, is reported.
func (q *Queue) Every(interval time.Duration, kind string, args any) {
	raw, err := json.Marshal(args)
	if err != nil {
		q.logger.Error("jobs: encode periodic args", "kind", kind, "error", err)
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		q.logger.ErrorContext(context.Background(), "jobs: Every called after Start; ignored", "kind", kind)
		return
	}
	if interval <= 0 {
		q.logger.ErrorContext(context.Background(), "jobs: Every needs a positive interval", "kind", kind, "interval", interval)
		return
	}
	for _, p := range q.periods {
		if p.args.Name == kind {
			q.logger.ErrorContext(context.Background(), "jobs: periodic kind registered twice", "kind", kind)
			return
		}
	}
	q.periods = append(q.periods, period{
		interval: interval,
		args:     jobArgs{Name: kind, JSON: raw},
	})
}

// Enqueue inserts a job. Inside database.TxManager.Do it uses that
// transaction, so a rolled back request leaves no job behind (PLT-09).
func (q *Queue) Enqueue(ctx context.Context, kind string, args any, opts app.EnqueueOptions) error {
	if kind == "" {
		return errors.New("jobs: kind is required")
	}
	client, err := q.running()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("jobs: encode args for %s: %w", kind, err)
	}

	insertOpts := &river.InsertOpts{MaxAttempts: q.maxAttempts(kind)}
	if !opts.RunAt.IsZero() {
		insertOpts.ScheduledAt = opts.RunAt
	}
	job := jobArgs{Name: kind, JSON: raw}

	if tx, ok := database.TxFromContext(ctx); ok {
		_, err = client.InsertTx(ctx, tx, job, insertOpts)
	} else {
		_, err = client.Insert(ctx, job, insertOpts)
	}
	if err != nil {
		return fmt.Errorf("jobs: enqueue %s: %w", kind, err)
	}
	return nil
}

// Start builds the River client from the handlers and schedules recorded in
// Init, starts it and starts cancelling jobs whose kind has no worker here.
func (q *Queue) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return errors.New("jobs: already started")
	}
	handlers := maps.Clone(q.handlers)
	periods := append([]period(nil), q.periods...)
	sweepInterval := q.sweepInterval
	q.mu.Unlock()

	workers := river.NewWorkers()
	for kind, reg := range handlers {
		w := &worker{
			kind:    kind,
			handler: reg.handler,
			timeout: timeout(reg.opts),
			logger:  q.logger,
		}
		// AddWorkerArgs takes the args value explicitly, which is what lets one
		// Go type carry every dynamic kind. AddWorkerSafely derives the kind
		// from the zero value and would register an empty kind here. It panics
		// only on a duplicate kind, which the map above rules out.
		river.AddWorkerArgs(workers, jobArgs{Name: kind}, w)
	}

	if len(handlers) == 0 {
		river.AddWorkerArgs(workers, jobArgs{Name: noopKind}, &worker{
			kind:    noopKind,
			handler: func(context.Context, json.RawMessage) error { return nil },
			timeout: defaultTimeout,
			logger:  q.logger,
		})
	}

	periodicJobs := make([]*river.PeriodicJob, 0, len(periods))
	for _, p := range periods {
		reg, ok := handlers[p.args.Name]
		if !ok {
			return fmt.Errorf("jobs: periodic job %s has no handler", p.args.Name)
		}
		insertOpts := &river.InsertOpts{MaxAttempts: maxAttempts(reg.opts)}
		p := p
		periodicJobs = append(periodicJobs, river.NewPeriodicJob(
			river.PeriodicInterval(p.interval),
			func() (river.JobArgs, *river.InsertOpts) { return p.args, insertOpts },
			&river.PeriodicJobOpts{ID: p.args.Name},
		))
	}

	client, err := river.NewClient(riverpgxv5.New(q.pool), &river.Config{
		Logger:  q.logger,
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		PeriodicJobs: periodicJobs,
		// Leave unknown kinds in the queue untouched instead of letting a
		// producer fetch and fail them; the sweeper cancels them (PLT-09).
		FetchOnlyKnownKinds: true,
	})
	if err != nil {
		return fmt.Errorf("jobs: new client: %w", err)
	}

	// The server drains HTTP first and calls Stop afterwards, so the client
	// must not stop merely because the signal context was cancelled.
	lifecycle := context.WithoutCancel(ctx)
	if err := client.Start(lifecycle); err != nil {
		return fmt.Errorf("jobs: start client: %w", err)
	}

	// Cancel jobs left behind by a module that this server does not have.
	known := knownKinds(handlers)
	if err := q.sweep(ctx, client, known); err != nil && ctx.Err() == nil {
		q.logger.ErrorContext(ctx, "jobs: cancel jobs without a worker", "error", err)
	}

	sweepCtx, cancel := context.WithCancel(lifecycle)
	q.mu.Lock()
	q.client = client
	q.stopSweep = cancel
	q.started = true
	// Add while holding the mutex Stop uses, so its Wait cannot race with a
	// later Add.
	q.sweepWG.Add(1)
	q.mu.Unlock()
	go q.sweepLoop(sweepCtx, client, known, sweepInterval)
	return nil
}

// Stop stops fetching jobs, cancels the sweeper and waits for running jobs, so
// a shutdown does not cut a handler in half (NFR-11).
func (q *Queue) Stop(ctx context.Context) error {
	q.mu.Lock()
	if !q.started {
		q.mu.Unlock()
		return nil
	}
	cancel, client := q.stopSweep, q.client
	q.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	sweepDone := make(chan struct{})
	go func() {
		q.sweepWG.Wait()
		close(sweepDone)
	}()
	select {
	case <-sweepDone:
	case <-ctx.Done():
		return ctx.Err()
	}

	if client != nil {
		if err := client.Stop(ctx); err != nil {
			return fmt.Errorf("jobs: stop client: %w", err)
		}
	}

	q.mu.Lock()
	q.client = nil
	q.stopSweep = nil
	q.started = false
	q.mu.Unlock()
	return nil
}

// running returns the started client or an error that says why there is none.
func (q *Queue) running() (*river.Client[pgx.Tx], error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.started || q.client == nil {
		return nil, errors.New("jobs: not started")
	}
	return q.client, nil
}

// maxAttempts is the retry limit recorded on the job row at insert time.
func (q *Queue) maxAttempts(kind string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return maxAttempts(q.handlers[kind].opts)
}

func maxAttempts(opts app.JobOptions) int {
	if opts.MaxAttempts > 0 {
		return opts.MaxAttempts
	}
	return defaultMaxAttempts
}

func timeout(opts app.JobOptions) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return defaultTimeout
}

// knownKinds returns the kinds this server can handle.
func knownKinds(handlers map[string]registration) []string {
	known := make([]string, 0, len(handlers))
	for kind := range handlers {
		known = append(known, kind)
	}
	return known
}

// sweepLoop cancels unknown-kind jobs once per interval until Stop.
func (q *Queue) sweepLoop(ctx context.Context, client *river.Client[pgx.Tx], known []string, interval time.Duration) {
	defer q.sweepWG.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := q.sweep(ctx, client, known); err != nil && ctx.Err() == nil {
				q.logger.ErrorContext(ctx, "jobs: cancel jobs without a worker", "error", err)
			}
		}
	}
}

// sweep cancels every queued job whose kind has no handler on this server
// (PLT-09). River's FetchOnlyKnownKinds keeps such jobs out of a worker, so
// they stay available until this runs.
func (q *Queue) sweep(ctx context.Context, client *river.Client[pgx.Tx], known []string) error {
	for {
		ids, err := q.unknownJobIDs(ctx, known)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := client.JobCancel(ctx, id); err != nil {
				return fmt.Errorf("cancel job %d: %w", id, err)
			}
		}
		if len(ids) < sweepBatch {
			return nil
		}
	}
}

func (q *Queue) unknownJobIDs(ctx context.Context, known []string) ([]int64, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id FROM river_job
		WHERE state::text = ANY($1) AND NOT (kind = ANY($2))
		LIMIT $3`, queuedStates, known, sweepBatch)
	if err != nil {
		return nil, fmt.Errorf("list jobs without a worker: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan job id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs without a worker: %w", err)
	}
	return ids, nil
}

// worker runs one kind. River calls one worker per kind so the timeout from
// that kind's JobOptions applies.
type worker struct {
	river.WorkerDefaults[jobArgs]
	kind    string
	handler app.JobHandler
	timeout time.Duration
	logger  *slog.Logger
}

func (w *worker) Timeout(*river.Job[jobArgs]) time.Duration { return w.timeout }

// Work unwraps the JSON payload and calls the module's handler. A panic is
// turned into an error so River retries it like any other failure.
func (w *worker) Work(ctx context.Context, job *river.Job[jobArgs]) (err error) {
	defer func() {
		if p := recover(); p != nil {
			w.logger.ErrorContext(ctx, "jobs: handler panicked", "kind", w.kind, "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("jobs: handler for %s panicked: %v", w.kind, p)
		}
	}()
	if err := w.handler(ctx, job.Args.JSON); err != nil {
		return fmt.Errorf("jobs: %s: %w", w.kind, err)
	}
	return nil
}
