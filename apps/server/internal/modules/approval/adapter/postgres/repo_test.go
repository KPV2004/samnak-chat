package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/platform/database"
	dbmigrations "github.com/KPV2004/samnak-chat/apps/server/internal/platform/database/migrations"
)

var (
	dbOnce sync.Once
	dbURL  string
	dbErr  string
)

// testPool starts one Postgres container for the package, migrates it and
// hands out pooled connections. Tests isolate themselves with their own
// workspaces and users.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil && os.Getenv("DOCKER_HOST") == "" {
		t.Skip("skipping integration test: Docker is not available")
	}
	dbOnce.Do(func() {
		ctx := context.Background()
		c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("test"), tcpostgres.WithUsername("test"), tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
		)
		if err != nil {
			dbErr = fmt.Sprintf("cannot start postgres container: %v", err)
			return
		}
		url, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			dbErr = err.Error()
			return
		}
		db, err := sql.Open("pgx", url)
		if err != nil {
			dbErr = err.Error()
			return
		}
		defer func() { _ = db.Close() }()
		if err := dbmigrations.Run(ctx, db, "approval", migrations.FS); err != nil {
			dbErr = err.Error()
			return
		}
		dbURL = url
	})
	if dbErr != "" {
		t.Fatal(dbErr)
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

// at returns a time on a microsecond boundary: that is all Postgres keeps.
func at(base time.Time, plus time.Duration) time.Time {
	return base.Add(plus).UTC().Truncate(time.Microsecond)
}

var base = time.Date(2026, 11, 17, 9, 0, 0, 123456000, time.UTC)

type fixture struct {
	repo      *postgres.Repo
	pool      *pgxpool.Pool
	ws        string
	requester string
	approver  string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testPool(t)
	return fixture{repo: postgres.New(pool), pool: pool, ws: newID(), requester: newID(), approver: newID()}
}

func (f fixture) request(at time.Time) (domain.Request, domain.Step, domain.HistoryEntry) {
	id := newID()
	r := domain.Request{
		ID: id, WorkspaceID: f.ws, Type: domain.TypeLeave, RequesterID: f.requester, ApproverID: f.approver,
		Status: domain.StatusPending, CreatedAt: at, UpdatedAt: at,
		Payload: json.RawMessage(`{"leave_type":"annual","start_date":"2026-12-01","end_date":"2026-12-02","reason":"trip"}`),
	}
	step := domain.Step{RequestID: id, StepNo: 1, ApproverID: f.approver, Status: domain.StatusPending}
	h := domain.HistoryEntry{ID: newID(), RequestID: id, ActorID: f.requester, Action: domain.ActionCreated, At: at}
	return r, step, h
}

func (f fixture) insert(t *testing.T, at time.Time) domain.Request {
	t.Helper()
	r, step, h := f.request(at)
	if err := f.repo.InsertRequest(context.Background(), r, step, h); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestInsertAndGetRequest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	want := f.insert(t, at(base, 0))

	got, err := f.repo.Request(ctx, f.ws, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Type != domain.TypeLeave || got.Status != domain.StatusPending ||
		got.RequesterID != f.requester || got.ApproverID != f.approver {
		t.Fatalf("got %+v", got)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) || got.CreatedAt.Location() != time.UTC {
		t.Errorf("times = %v / %v, want %v in UTC", got.CreatedAt, got.UpdatedAt, want.CreatedAt)
	}
	var payload map[string]any
	if err := json.Unmarshal(got.Payload, &payload); err != nil || payload["leave_type"] != "annual" {
		t.Errorf("payload = %s, %v", got.Payload, err)
	}
	if got.MessageID != "" || got.ChannelID != "" || got.DecidedAt != nil || got.Reason != "" {
		t.Errorf("a new request has no card, decision or reason: %+v", got)
	}

	hist, err := f.repo.History(ctx, want.ID)
	if err != nil || len(hist) != 1 || hist[0].Action != domain.ActionCreated || hist[0].ActorID != f.requester {
		t.Fatalf("history = %+v, %v", hist, err)
	}
}

func TestRequestIsScopedToItsWorkspace(t *testing.T) {
	f := newFixture(t)
	r := f.insert(t, at(base, 0))
	if _, err := f.repo.Request(context.Background(), newID(), r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("another workspace: %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Request(context.Background(), f.ws, "not-a-uuid"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("bad ID: %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Request(context.Background(), f.ws, newID()); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown ID: %v, want ErrNotFound", err)
	}
}

func TestInsertRequestRollsBackWithItsTransaction(t *testing.T) {
	f := newFixture(t)
	tx := database.NewTxManager(f.pool)
	r, step, h := f.request(at(base, 0))
	boom := errors.New("boom")
	err := tx.Do(context.Background(), func(ctx context.Context) error {
		if err := f.repo.InsertRequest(ctx, r, step, h); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Do = %v", err)
	}
	if _, err := f.repo.Request(context.Background(), f.ws, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("request survived the rollback: %v", err)
	}
}

func TestListFiltersAndPages(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := newID()

	// Five requests by the requester to the approver, one by the approver to
	// the requester, one between two other people.
	var mine []domain.Request
	for i := range 5 {
		mine = append(mine, f.insert(t, at(base, time.Duration(i)*time.Minute)))
	}
	reverse := f
	reverse.requester, reverse.approver = f.approver, f.requester
	back := reverse.insert(t, at(base, 10*time.Minute))
	third := f
	third.requester, third.approver = other, newID()
	third.insert(t, at(base, 11*time.Minute))

	ids := func(rs []domain.Request) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.ID
		}
		return out
	}
	list := func(role string, statuses []domain.Status, before string, limit int) []domain.Request {
		t.Helper()
		rs, err := f.repo.List(ctx, usecase.ListFilter{
			WorkspaceID: f.ws, UserID: f.requester, Role: role, Statuses: statuses, BeforeID: before, Limit: limit,
		})
		if err != nil {
			t.Fatal(err)
		}
		return rs
	}

	asRequester := list("requester", nil, "", 50)
	if len(asRequester) != 5 || asRequester[0].ID != mine[4].ID || asRequester[4].ID != mine[0].ID {
		t.Errorf("requester list = %v, want the five newest first", ids(asRequester))
	}
	asApprover := list("approver", nil, "", 50)
	if len(asApprover) != 1 || asApprover[0].ID != back.ID {
		t.Errorf("approver list = %v, want only the request sent to the user", ids(asApprover))
	}
	if both := list("", nil, "", 50); len(both) != 6 || both[0].ID != back.ID {
		t.Errorf("both roles = %v, want six, newest first", ids(both))
	}

	page1 := list("requester", nil, "", 2)
	page2 := list("requester", nil, page1[1].ID, 2)
	page3 := list("requester", nil, page2[1].ID, 2)
	if got := append(append(ids(page1), ids(page2)...), ids(page3)...); fmt.Sprint(got) != fmt.Sprint(ids(asRequester)) {
		t.Errorf("pages = %v, want %v", got, ids(asRequester))
	}
	if _, err := f.repo.List(ctx, usecase.ListFilter{WorkspaceID: f.ws, UserID: f.requester, BeforeID: newID(), Limit: 5}); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("unknown cursor: %v, want ErrNotFound", err)
	}

	if _, changed, err := f.repo.Decide(ctx, f.ws, mine[1].ID, domain.StatusApproved, "", at(base, time.Hour)); err != nil || !changed {
		t.Fatalf("Decide = %v, %v", changed, err)
	}
	if got := list("requester", []domain.Status{domain.StatusApproved}, "", 50); len(got) != 1 || got[0].ID != mine[1].ID {
		t.Errorf("approved filter = %v", ids(got))
	}
	if got := list("requester", []domain.Status{domain.StatusPending, domain.StatusRejected}, "", 50); len(got) != 4 {
		t.Errorf("pending+rejected filter = %v, want 4", ids(got))
	}
}

func TestDecideChangesAPendingRequestOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r := f.insert(t, at(base, 0))
	decidedAt := at(base, time.Hour)

	got, changed, err := f.repo.Decide(ctx, f.ws, r.ID, domain.StatusRejected, "no budget", decidedAt)
	if err != nil || !changed {
		t.Fatalf("Decide = %v, %v", changed, err)
	}
	if got.Status != domain.StatusRejected || got.Reason != "no budget" || got.DecidedAt == nil || !got.DecidedAt.Equal(decidedAt) || !got.UpdatedAt.Equal(decidedAt) {
		t.Errorf("decided request = %+v", got)
	}

	again, changed, err := f.repo.Decide(ctx, f.ws, r.ID, domain.StatusApproved, "", at(base, 2*time.Hour))
	if err != nil || changed {
		t.Fatalf("second Decide = %v, %v, want unchanged", changed, err)
	}
	if again.Status != domain.StatusRejected {
		t.Errorf("second decide returned %+v, want the stored rejected request", again)
	}

	var stepStatus, stepReason string
	err = f.pool.QueryRow(ctx, `SELECT status, reason FROM approval_steps WHERE request_id = $1 AND step_no = 1`, r.ID).Scan(&stepStatus, &stepReason)
	if err != nil || stepStatus != "rejected" || stepReason != "no budget" {
		t.Errorf("step = %q %q, %v", stepStatus, stepReason, err)
	}

	if _, _, err := f.repo.Decide(ctx, newID(), r.ID, domain.StatusApproved, "", decidedAt); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("Decide in another workspace: %v, want ErrNotFound", err)
	}
}

func TestHistoryIsOrderedByTime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r := f.insert(t, at(base, 0))
	later := domain.HistoryEntry{ID: newID(), RequestID: r.ID, ActorID: f.approver, Action: domain.ActionApproved, Reason: "ok", At: at(base, time.Minute)}
	if err := f.repo.InsertHistory(ctx, later); err != nil {
		t.Fatal(err)
	}
	hist, err := f.repo.History(ctx, r.ID)
	if err != nil || len(hist) != 2 {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	if hist[0].Action != domain.ActionCreated || hist[1].Action != domain.ActionApproved || hist[1].Reason != "ok" || !hist[1].At.Equal(later.At) {
		t.Errorf("history = %+v", hist)
	}
}

func TestSetCardOnlyOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r := f.insert(t, at(base, 0))
	ch, msg := newID(), newID()

	if changed, err := f.repo.SetCard(ctx, f.ws, r.ID, ch, msg); err != nil || !changed {
		t.Fatalf("first SetCard = %v, %v", changed, err)
	}
	if changed, err := f.repo.SetCard(ctx, f.ws, r.ID, newID(), newID()); err != nil || changed {
		t.Fatalf("second SetCard = %v, %v, want unchanged", changed, err)
	}
	got, err := f.repo.Request(ctx, f.ws, r.ID)
	if err != nil || got.MessageID != msg || got.ChannelID != ch {
		t.Fatalf("request = %+v, %v, want the first card kept", got, err)
	}
	if !got.UpdatedAt.Equal(r.UpdatedAt) {
		t.Errorf("storing the card changed updated_at to %v: the card version would move without a decision", got.UpdatedAt)
	}
}

func TestAttachments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	mk := func(uploader string, plus time.Duration) domain.Attachment {
		id := newID()
		a := domain.Attachment{
			ID: id, WorkspaceID: f.ws, UploaderID: uploader, ObjectKey: domain.ObjectKey(f.ws, id),
			FileName: "cert.pdf", Size: 2048, MimeType: "application/pdf", CreatedAt: at(base, plus),
		}
		if err := f.repo.InsertAttachment(ctx, a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	a1, a2 := mk(f.requester, 0), mk(f.requester, time.Second)
	theirs := mk(f.approver, 0)

	got, err := f.repo.Attachment(ctx, f.ws, a1.ID)
	if err != nil || got.ID != a1.ID || got.RequestID != "" || got.ObjectKey != a1.ObjectKey || got.Size != 2048 || !got.CreatedAt.Equal(a1.CreatedAt) {
		t.Fatalf("Attachment = %+v, %v", got, err)
	}
	if _, err := f.repo.Attachment(ctx, newID(), a1.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("other workspace: %v", err)
	}
	list, err := f.repo.AttachmentsByIDs(ctx, f.ws, []string{a2.ID, a1.ID})
	if err != nil || len(list) != 2 || list[0].ID != a1.ID {
		t.Fatalf("AttachmentsByIDs = %+v, %v", list, err)
	}

	r := f.insert(t, at(base, time.Minute))
	// Someone else's file is not linked, so the count falls short.
	n, err := f.repo.LinkAttachments(ctx, f.ws, r.ID, f.requester, []string{a1.ID, a2.ID, theirs.ID})
	if err != nil || n != 2 {
		t.Fatalf("LinkAttachments = %d, %v, want 2", n, err)
	}
	// A linked file is not linked again, even by its uploader.
	other := f.insert(t, at(base, 2*time.Minute))
	if n, err := f.repo.LinkAttachments(ctx, f.ws, other.ID, f.requester, []string{a1.ID}); err != nil || n != 0 {
		t.Fatalf("relinking = %d, %v, want 0", n, err)
	}
	files, err := f.repo.AttachmentsOf(ctx, r.ID)
	if err != nil || len(files) != 2 || files[0].ID != a1.ID || files[1].ID != a2.ID || files[0].RequestID != r.ID {
		t.Fatalf("AttachmentsOf = %+v, %v", files, err)
	}
}
