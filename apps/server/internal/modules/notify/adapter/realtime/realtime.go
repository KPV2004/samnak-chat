// Package realtime turns notify use-case events into WebSocket frames.
// Delivery is best effort: the database is the source of truth and clients
// refetch the list and the count after reconnecting.
package realtime

import (
	"context"
	"log/slog"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/usecase"
)

// Publisher implements usecase.Events on app.Realtime.
type Publisher struct {
	rt  app.Realtime // nil in tests and in the openapi command
	log *slog.Logger
}

var _ usecase.Events = (*Publisher)(nil)

func New(rt app.Realtime, log *slog.Logger) *Publisher { return &Publisher{rt: rt, log: log} }

// send delivers env to the user's open connections. Nothing to do when
// realtime is not configured or the user is offline: they find the rows in
// the list.
func (p *Publisher) send(ctx context.Context, userID, typ string, data any) {
	if p.rt == nil {
		return
	}
	env, err := app.NewEnvelope(typ, data)
	if err != nil {
		p.log.ErrorContext(ctx, "notify: encode event", "type", typ, "error", err)
		return
	}
	if err := p.rt.SendToUsers(ctx, []string{userID}, env); err != nil {
		p.log.WarnContext(ctx, "notify: realtime delivery failed", "type", typ, "error", err)
	}
}

func (p *Publisher) Created(ctx context.Context, n domain.Notification, unreadCount int64) {
	p.send(ctx, n.UserID, wire.EventCreated, wire.CreatedEvent{
		WorkspaceID: n.WorkspaceID, Notification: wire.ToNotification(n), UnreadCount: unreadCount,
	})
}

func (p *Publisher) Read(ctx context.Context, workspaceID, userID string, ids []string, all bool, unreadCount int64) {
	if ids == nil {
		ids = []string{}
	}
	p.send(ctx, userID, wire.EventRead, wire.ReadEvent{
		WorkspaceID: workspaceID, IDs: ids, All: all, UnreadCount: unreadCount,
	})
}
