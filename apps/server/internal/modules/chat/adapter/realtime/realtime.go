// Package realtime turns chat use-case events into WebSocket frames and bus
// events. Delivery is best effort: the database is the source of truth and
// clients catch up over REST.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/adapter/wire"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
)

// Publisher implements usecase.Events.
type Publisher struct {
	rt      app.Realtime // nil in tests and in the openapi command
	log     *slog.Logger
	publish func(ctx context.Context, ch domain.Channel, m usecase.MessageView)
}

var _ usecase.Events = (*Publisher)(nil)

// New returns a Publisher. onCreated is called for every new message to
// publish chat.message_created; it may be nil.
func New(rt app.Realtime, log *slog.Logger, onCreated func(ctx context.Context, ch domain.Channel, m usecase.MessageView)) *Publisher {
	return &Publisher{rt: rt, log: log, publish: onCreated}
}

func (p *Publisher) send(ctx context.Context, typ string, recipients []string, data any) {
	if p.rt == nil || len(recipients) == 0 {
		return
	}
	env, err := app.NewEnvelope(typ, data)
	if err != nil {
		p.log.ErrorContext(ctx, "chat: encode event", "type", typ, "error", err)
		return
	}
	if err := p.rt.SendToUsers(ctx, recipients, env); err != nil {
		p.log.WarnContext(ctx, "chat: realtime delivery failed", "type", typ, "error", err)
	}
}

type messageData struct {
	WorkspaceID string       `json:"workspace_id"`
	ChannelID   string       `json:"channel_id"`
	Message     wire.Message `json:"message"`
}

func messageEvent(m usecase.MessageView, viewerID string) messageData {
	return messageData{WorkspaceID: m.WorkspaceID, ChannelID: m.ChannelID, Message: wire.ToMessage(m, viewerID)}
}

// sendMessage delivers a message event. Reactions say whether the viewer used
// an emoji, so a message that has some goes out once per recipient.
func (p *Publisher) sendMessage(ctx context.Context, typ string, m usecase.MessageView, recipients []string) {
	if len(m.Reactions) == 0 {
		p.send(ctx, typ, recipients, messageEvent(m, ""))
		return
	}
	for _, r := range recipients {
		p.send(ctx, typ, []string{r}, messageEvent(m, r))
	}
}

func (p *Publisher) MessageCreated(ctx context.Context, ch domain.Channel, m usecase.MessageView, recipients []string) {
	p.sendMessage(ctx, "message.created", m, recipients)
	if p.publish != nil {
		p.publish(ctx, ch, m)
	}
}

func (p *Publisher) MessageUpdated(ctx context.Context, m usecase.MessageView, recipients []string) {
	p.sendMessage(ctx, "message.updated", m, recipients)
}

func (p *Publisher) MessageDeleted(ctx context.Context, m usecase.MessageView, recipients []string) {
	p.sendMessage(ctx, "message.deleted", m, recipients)
}

type cardData struct {
	WorkspaceID string          `json:"workspace_id"`
	ChannelID   string          `json:"channel_id"`
	MessageID   string          `json:"message_id"`
	Payload     json.RawMessage `json:"payload"`
}

func (p *Publisher) CardUpdated(ctx context.Context, m usecase.MessageView, recipients []string) {
	p.send(ctx, "card.updated", recipients, cardData{WorkspaceID: m.WorkspaceID, ChannelID: m.ChannelID, MessageID: m.ID, Payload: m.Payload})
}

type readData struct {
	WorkspaceID  string `json:"workspace_id"`
	ChannelID    string `json:"channel_id"`
	LastReadSeq  int64  `json:"last_read_seq"`
	UnreadCount  int64  `json:"unread_count"`
	MentionCount int64  `json:"mention_count"`
}

func (p *Publisher) ChannelRead(ctx context.Context, v usecase.ChannelView, userID string) {
	p.send(ctx, "channel.read", []string{userID}, readData{
		WorkspaceID: v.WorkspaceID, ChannelID: v.ID, LastReadSeq: v.LastReadSeq, UnreadCount: v.UnreadCount, MentionCount: v.MentionCount,
	})
}

// channelData holds what is the same for every recipient. Per-user counts are
// left out: each client keeps its own and refetches the channel when it needs
// them.
type channelData struct {
	WorkspaceID string     `json:"workspace_id"`
	ChannelID   string     `json:"channel_id"`
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Topic       string     `json:"topic"`
	LastSeq     int64      `json:"last_seq"`
	MemberCount int64      `json:"member_count"`
	DMUserIDs   []string   `json:"dm_user_ids"`
	ArchivedAt  *time.Time `json:"archived_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (p *Publisher) ChannelUpdated(ctx context.Context, v usecase.ChannelView, recipients []string) {
	dm := v.DMUserIDs
	if dm == nil {
		dm = []string{}
	}
	p.send(ctx, "channel.updated", recipients, channelData{
		WorkspaceID: v.WorkspaceID, ChannelID: v.ID, Type: string(v.Type), Name: v.Name, Topic: v.Topic, LastSeq: v.LastSeq,
		MemberCount: v.MemberCount, DMUserIDs: dm, ArchivedAt: v.ArchivedAt, CreatedAt: v.CreatedAt.UTC(),
	})
}
