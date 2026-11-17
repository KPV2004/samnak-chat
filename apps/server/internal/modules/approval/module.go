package approval

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	httpadapter "github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/adapter/http"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/adapter/postgres"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/migrations"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/usecase"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/auth"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/workspace"
)

// Migrations is the goose migration set of this module, to run with
// migrations.Run(ctx, db, "approval", approval.Migrations).
var Migrations = migrations.FS

// Module is the approval module. It is optional: with it off, no request
// exists and its cards in chat show their fallback text (PLT-11).
type Module struct {
	services *app.Services
	uc       *usecase.Service
	log      *slog.Logger
}

var _ app.Module = (*Module)(nil)

func New() *Module { return &Module{} }

func (*Module) Name() string { return "approval" }

// Requires is empty on purpose: auth, workspace and chat are core modules and
// always on, and every service approval takes from them has a safe fallback.
func (*Module) Requires() []string { return nil }

// Provide exposes nothing: other modules listen to the events in events.go.
func (*Module) Provide(*app.Services) {}

// Init builds the use cases, registers the card kind, the notification rules
// and the event types. It does no I/O: with no "db" in d.Platform (the openapi
// command) the repository fails when called, which never happens because no
// request is served.
func (m *Module) Init(d *app.Deps) error {
	m.log = d.Logger
	if m.log == nil {
		m.log = slog.Default()
	}
	m.services = d.Services
	pool, _ := d.Platform["db"].(*pgxpool.Pool)

	var cards usecase.Cards
	if reg, ok := app.Lookup[app.CardRegistry](d.Services); ok {
		reg.Register(app.CardKind{Kind: usecase.CardKind, Module: "approval"})
		cards = cardPoster{reg: reg}
	}
	m.uc = usecase.New(usecase.Deps{
		Tx: d.Tx, Repo: postgres.New(pool), Bus: d.Bus,
		Members: members{d.Services}, Managers: managers{d.Services}, Names: names{d.Services},
		Cards: cards, Storage: d.Storage, Log: m.log, Now: time.Now,
	})
	if d.Bus != nil {
		// These events are published inside transactions, so the outbox must
		// know how to decode them (PLT-10).
		d.Bus.Declare(domain.RequestCreated{}, domain.RequestApproved{}, domain.RequestRejected{}, domain.RequestCancelled{})
	}
	if d.Notifications != nil {
		// notify reads the rules in Subscribe, after every Init ran. With a
		// no-op registry these lines do nothing and approval works as before
		// (NOTI-01, NOTI-05).
		for _, name := range eventNames {
			d.Notifications.On(name, m.uc.Notify)
		}
	}
	return nil
}

var eventNames = []string{
	domain.RequestCreated{}.EventName(), domain.RequestApproved{}.EventName(),
	domain.RequestRejected{}.EventName(), domain.RequestCancelled{}.EventName(),
}

func (m *Module) Routes(api huma.API) {
	httpadapter.New(m.uc, m.log).Register(api, m.services)
}

// Subscribe keeps the card in step with the request: every approval event
// posts the card when there is none and rewrites it otherwise (APR-03). The
// handler works from the stored row, so a late or repeated event is safe.
func (m *Module) Subscribe(bus app.EventBus) {
	if bus == nil || m.uc == nil {
		return
	}
	for _, name := range eventNames {
		bus.Subscribe(name, m.syncCard)
	}
}

func (m *Module) syncCard(ctx context.Context, evt app.Event) error {
	var workspaceID, requestID string
	switch e := evt.(type) {
	case domain.RequestCreated:
		workspaceID, requestID = e.WorkspaceID, e.RequestID
	case domain.RequestApproved:
		workspaceID, requestID = e.WorkspaceID, e.RequestID
	case domain.RequestRejected:
		workspaceID, requestID = e.WorkspaceID, e.RequestID
	case domain.RequestCancelled:
		workspaceID, requestID = e.WorkspaceID, e.RequestID
	default:
		return nil
	}
	// The change is committed; a client that hung up must not stop the card.
	return m.uc.SyncCard(context.WithoutCancel(ctx), workspaceID, requestID)
}

// cardPoster adapts chat's card registry: the card goes into the DM between
// the two people, sent as the requester (APR-03).
type cardPoster struct{ reg app.CardRegistry }

func (c cardPoster) Post(ctx context.Context, workspaceID, requesterID, approverID string, payload any, fallback string) (string, string, error) {
	channelID, err := c.reg.DirectChannel(ctx, workspaceID, []string{requesterID, approverID})
	if err != nil {
		return "", "", err
	}
	messageID, err := c.reg.Post(ctx, app.Card{
		WorkspaceID: workspaceID, ChannelID: channelID, SenderID: requesterID,
		Kind: usecase.CardKind, Payload: payload, FallbackText: fallback,
	})
	if err != nil {
		return "", "", err
	}
	return channelID, messageID, nil
}

func (c cardPoster) Update(ctx context.Context, workspaceID, messageID string, payload any) error {
	return c.reg.Update(ctx, workspaceID, messageID, payload)
}

// members adapts workspace's membership resolver: a user who is not an active
// member, or any failure, is not a valid approver.
type members struct{ s *app.Services }

func (m members) Active(ctx context.Context, workspaceID, userID string) bool {
	r, ok := app.Lookup[app.MembershipResolver](m.s)
	if !ok {
		return false
	}
	_, err := r.Role(ctx, workspaceID, userID)
	return err == nil
}

// managers adapts workspace's group directory; without it nobody has a
// default approver and the requester must choose one (APR-02).
type managers struct{ s *app.Services }

func (m managers) ManagersOf(ctx context.Context, workspaceID, userID string) ([]string, error) {
	g, ok := app.Lookup[workspace.GroupDirectory](m.s)
	if !ok {
		return nil, nil
	}
	return g.ManagersOf(ctx, workspaceID, userID)
}

// names adapts auth's directory; without it names are empty.
type names struct{ s *app.Services }

func (n names) Names(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	dir, ok := app.Lookup[auth.Directory](n.s)
	if !ok {
		return out
	}
	users, err := dir.Users(ctx, ids)
	if err != nil {
		return out
	}
	for _, u := range users {
		out[u.ID] = u.DisplayName
	}
	return out
}
