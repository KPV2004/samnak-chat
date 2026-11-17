package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	// notifyChannel is what Publish notifies and the relay listens on.
	notifyChannel = "platform_outbox"
	// defaultPollInterval is the safety net when a notification is missed,
	// for example while the LISTEN connection is being re-established.
	defaultPollInterval = 2 * time.Second
	// claimBatch bounds one claim transaction.
	claimBatch = 100
	// listenRetry is the wait before the LISTEN connection is rebuilt.
	listenRetry = time.Second
	// retention is how long published rows are kept before pruning.
	retention = 7 * 24 * time.Hour
	// pruneKind is the job that deletes old published rows.
	pruneKind     = "platform.outbox_prune"
	pruneInterval = 24 * time.Hour
)

// outbox is the state a Bus needs only when it has a database.
type outbox struct {
	pool *pgxpool.Pool

	declMu   sync.RWMutex
	declared map[string]reflect.Type

	relayMu      sync.Mutex
	pollInterval time.Duration
	relay        *relay
}

// relay is one running Start.
type relay struct {
	stopLoop  context.CancelFunc // asks the goroutines to stop between claims
	hardStop  context.CancelFunc // cancels a claim in progress
	done      chan struct{}
	warnedFor map[string]bool // only touched by the relay goroutine
}

// NewOutbox returns a Bus that writes events published inside a transaction
// to platform_outbox_events (PLT-10). Start must be called after every module
// has subscribed and declared its events.
func NewOutbox(pool *pgxpool.Pool, log *slog.Logger) *Bus {
	b := New(log)
	b.pool = pool
	b.pollInterval = defaultPollInterval
	return b
}

// Declare records the type of each event so the relay can decode a stored
// event back into it. It does nothing on a bus without a database.
func (b *Bus) Declare(protos ...app.Event) {
	if b.pool == nil {
		return
	}
	b.declMu.Lock()
	defer b.declMu.Unlock()
	if b.declared == nil {
		b.declared = map[string]reflect.Type{}
	}
	for _, p := range protos {
		if p == nil {
			continue
		}
		name, t := p.EventName(), reflect.TypeOf(p)
		if prev, ok := b.declared[name]; ok && prev != t {
			b.log.Error("eventbus: event name declared with two types", "event", name, "kept", prev.String(), "ignored", t.String())
			continue
		}
		b.declared[name] = t
	}
}

// SetPollInterval changes how often the relay polls. It exists for tests; the
// default of two seconds matches the plan. A call after Start is ignored.
func (b *Bus) SetPollInterval(d time.Duration) {
	b.relayMu.Lock()
	defer b.relayMu.Unlock()
	if d <= 0 || b.relay != nil {
		b.log.Error("eventbus: SetPollInterval needs a positive duration before Start", "interval", d)
		return
	}
	b.pollInterval = d
}

// enqueue writes evt to the outbox in tx. Postgres delivers the notification
// only if the transaction commits, so a rollback leaves nothing behind.
func (b *Bus) enqueue(ctx context.Context, tx pgx.Tx, evt app.Event) error {
	name := evt.EventName()
	b.declMu.RLock()
	want, ok := b.declared[name]
	b.declMu.RUnlock()
	if !ok {
		return fmt.Errorf("eventbus: event %q is published in a transaction but not declared; call Declare in Init", name)
	}
	if got := reflect.TypeOf(evt); got != want {
		return fmt.Errorf("eventbus: event %q is declared as %s but published as %s", name, want, got)
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("eventbus: encode %q: %w", name, err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("eventbus: new id: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO platform_outbox_events (id, name, payload) VALUES ($1, $2, $3)`,
		id, name, payload); err != nil {
		return fmt.Errorf("eventbus: write %q to the outbox: %w", name, err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, '')`, notifyChannel); err != nil {
		return fmt.Errorf("eventbus: notify: %w", err)
	}
	return nil
}

// Start runs the relay: it listens for new outbox rows, also polls, and
// delivers each committed row to the local handlers. Call it after the modules
// have subscribed. It fails on a bus without a database.
func (b *Bus) Start(ctx context.Context) error {
	if b.pool == nil {
		return errors.New("eventbus: Start needs a database; use NewOutbox")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.relayMu.Lock()
	defer b.relayMu.Unlock()
	if b.relay != nil {
		return errors.New("eventbus: already started")
	}

	// The server drains HTTP first and calls Stop afterwards, so the relay must
	// not stop merely because the signal context was cancelled.
	lifecycle, hardStop := context.WithCancel(context.WithoutCancel(ctx))
	loopCtx, stopLoop := context.WithCancel(lifecycle)
	r := &relay{stopLoop: stopLoop, hardStop: hardStop, done: make(chan struct{}), warnedFor: map[string]bool{}}
	b.relay = r

	wake := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.listen(loopCtx, wake)
	}()
	poll := b.pollInterval
	go func() {
		b.run(lifecycle, loopCtx, r, wake, poll)
		wg.Wait()
		close(r.done)
	}()
	return nil
}

// Stop stops listening, waits for the claim in progress (its handlers and its
// commit) and returns. When ctx ends first the claim is cancelled, which
// leaves its events for redelivery.
func (b *Bus) Stop(ctx context.Context) error {
	b.relayMu.Lock()
	r := b.relay
	b.relayMu.Unlock()
	if r == nil {
		return nil
	}
	r.stopLoop()
	select {
	case <-r.done:
	case <-ctx.Done():
		r.hardStop()
		<-r.done
		b.clearRelay(r)
		return fmt.Errorf("eventbus: stop: %w", ctx.Err())
	}
	r.hardStop()
	b.clearRelay(r)
	return nil
}

func (b *Bus) clearRelay(r *relay) {
	b.relayMu.Lock()
	defer b.relayMu.Unlock()
	if b.relay == r {
		b.relay = nil
	}
}

// run drains the outbox on every wake-up and every poll until loopCtx ends.
// Claims use lifecycle, which only a hard stop cancels, so Stop never cuts a
// handler in half.
func (b *Bus) run(lifecycle, loopCtx context.Context, r *relay, wake <-chan struct{}, poll time.Duration) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	polled := true
	for {
		b.drain(lifecycle, loopCtx)
		if polled {
			b.warnUndeclared(lifecycle, r)
		}
		select {
		case <-loopCtx.Done():
			return
		case <-wake:
			polled = false
		case <-ticker.C:
			polled = true
		}
	}
}

// drain claims batches until the outbox has nothing more for this instance.
func (b *Bus) drain(ctx, loopCtx context.Context) {
	for loopCtx.Err() == nil {
		n, err := b.claim(ctx)
		if err != nil {
			if ctx.Err() == nil {
				b.log.ErrorContext(ctx, "eventbus: relay claim failed; events stay in the outbox", "error", err)
			}
			return
		}
		if n < claimBatch {
			return
		}
	}
}

// pending is a claimed row.
type pending struct {
	id      pgtype.UUID
	name    string
	payload []byte
}

// claim delivers up to claimBatch unpublished events of declared types and
// marks them published, all in one transaction. FOR UPDATE SKIP LOCKED keeps
// two relays from taking the same row. When the commit fails the handlers have
// already run, so the events are delivered again: delivery is at least once.
func (b *Bus) claim(ctx context.Context) (int, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	rows, err := tx.Query(ctx, `
		SELECT id, name, payload FROM platform_outbox_events
		WHERE published_at IS NULL AND name = ANY($1)
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $2`, b.declaredNames(), claimBatch)
	if err != nil {
		return 0, fmt.Errorf("select: %w", err)
	}
	batch, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pending, error) {
		var p pending
		err := row.Scan(&p.id, &p.name, &p.payload)
		return p, err
	})
	if err != nil {
		return 0, fmt.Errorf("scan: %w", err)
	}
	if len(batch) == 0 {
		return 0, nil
	}

	ids := make([]pgtype.UUID, 0, len(batch))
	for _, p := range batch {
		ids = append(ids, p.id)
		evt, err := b.decode(p.name, p.payload)
		if err != nil {
			// Retrying cannot fix a payload that does not decode.
			b.log.ErrorContext(ctx, "eventbus: decode outbox event; skipped", "event", p.name, "id", uuid.UUID(p.id.Bytes).String(), "error", err)
			continue
		}
		// The handlers get ctx without the claim transaction, so what they
		// publish or write is independent of the mark below.
		b.deliver(ctx, evt)
	}

	if _, err := tx.Exec(ctx, `UPDATE platform_outbox_events SET published_at = now() WHERE id = ANY($1)`, ids); err != nil {
		return 0, fmt.Errorf("mark published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(batch), nil
}

// declaredNames is never nil, so "name = ANY" is false rather than NULL when
// nothing is declared.
func (b *Bus) declaredNames() []string {
	b.declMu.RLock()
	defer b.declMu.RUnlock()
	names := make([]string, 0, len(b.declared))
	for name := range b.declared {
		names = append(names, name)
	}
	return names
}

// decode turns a stored payload back into the declared type of name.
func (b *Bus) decode(name string, payload []byte) (app.Event, error) {
	b.declMu.RLock()
	t, ok := b.declared[name]
	b.declMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("event %q is not declared", name)
	}
	if t.Kind() == reflect.Pointer {
		v := reflect.New(t.Elem())
		if err := json.Unmarshal(payload, v.Interface()); err != nil {
			return nil, fmt.Errorf("decode %s: %w", t, err)
		}
		evt, _ := v.Interface().(app.Event)
		return evt, nil
	}
	v := reflect.New(t)
	if err := json.Unmarshal(payload, v.Interface()); err != nil {
		return nil, fmt.Errorf("decode %s: %w", t, err)
	}
	evt, _ := v.Elem().Interface().(app.Event)
	return evt, nil
}

// warnUndeclared reports, once per name, unpublished events this instance
// cannot decode. They stay in the outbox for an instance that declares them,
// and the claim query skips them so they never block newer events.
func (b *Bus) warnUndeclared(ctx context.Context, r *relay) {
	rows, err := b.pool.Query(ctx,
		`SELECT DISTINCT name FROM platform_outbox_events WHERE published_at IS NULL AND NOT (name = ANY($1))`,
		b.declaredNames())
	if err != nil {
		return
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return
	}
	for _, name := range names {
		if r.warnedFor[name] {
			continue
		}
		r.warnedFor[name] = true
		b.log.ErrorContext(ctx, "eventbus: outbox holds an event no module here declared; left unpublished", "event", name)
	}
}

// listen keeps a LISTEN connection and signals wake on every notification.
// It reconnects after a failure; polling covers the gap.
func (b *Bus) listen(ctx context.Context, wake chan<- struct{}) {
	for ctx.Err() == nil {
		if err := b.listenOnce(ctx, wake); err != nil && ctx.Err() == nil {
			b.log.WarnContext(ctx, "eventbus: LISTEN connection lost; polling until it is back", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(listenRetry):
		}
	}
}

func (b *Bus) listenOnce(ctx context.Context, wake chan<- struct{}) error {
	pc, err := b.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	// Hijack hands the connection over so it neither counts against the pool
	// nor goes back with a LISTEN still active.
	conn := pc.Hijack()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	signal(wake) // events committed before the LISTEN took effect
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		signal(wake)
	}
}

func signal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default: // a wake-up is already pending
	}
}

// RegisterJobs registers the job that prunes published rows. Call it before
// the queue starts. A nil queue (tests, `server openapi`) registers nothing.
func (b *Bus) RegisterJobs(j app.Jobs) {
	if j == nil || b.pool == nil {
		return
	}
	j.Handle(pruneKind, b.prune, app.JobOptions{})
	j.Every(pruneInterval, pruneKind, struct{}{})
}

// prune deletes rows published more than seven days ago. Unpublished rows are
// never deleted: they wait for an instance that can deliver them.
func (b *Bus) prune(ctx context.Context, _ json.RawMessage) error {
	tag, err := b.pool.Exec(ctx,
		`DELETE FROM platform_outbox_events WHERE published_at < now() - $1::interval`,
		pgtype.Interval{Microseconds: retention.Microseconds(), Valid: true})
	if err != nil {
		return fmt.Errorf("eventbus: prune outbox: %w", err)
	}
	if n := tag.RowsAffected(); n > 0 {
		b.log.InfoContext(ctx, "eventbus: pruned outbox", "rows", n)
	}
	return nil
}

var _ app.EventBus = (*Bus)(nil)
