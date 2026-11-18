package usecase

import (
	"context"
	"fmt"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/helpdesk/domain"
)

// CardKind is the chat card kind of a ticket (HD-08).
const CardKind = "helpdesk.ticket"

// CardPayload is the payload of a helpdesk.ticket card. It carries what the web
// needs to draw the card without another request. Version is the ticket's
// updated_at in Unix milliseconds, so a late card.updated never overwrites a
// newer one on the client. CanClaimQueueID is the queue whose agents may claim
// the ticket while it is unassigned; null otherwise.
type CardPayload struct {
	TicketID        string  `json:"ticket_id"`
	Key             string  `json:"key"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`
	Priority        string  `json:"priority"`
	QueueName       string  `json:"queue_name"`
	RequesterID     string  `json:"requester_id"`
	RequesterName   string  `json:"requester_name"`
	AssigneeID      *string `json:"assignee_id"`
	AssigneeName    string  `json:"assignee_name"`
	CanClaimQueueID *string `json:"can_claim_queue_id"`
	Version         int64   `json:"version"`
}

// cardSlot is one of the two cards of a ticket: the one in the requester's
// channel and the one in the queue's agents' channel (HD-08).
type cardSlot struct {
	// channel is where a new card goes; empty means the ticket has no card
	// of this kind.
	channel func(t domain.Ticket, q domain.Queue) string
	// message is the stored message ID of the card, empty until it is posted.
	message func(t domain.Ticket) string
	// store records a posted message ID, only when none is stored yet.
	store func(ctx context.Context, s *Service, t domain.Ticket, messageID string) (bool, error)
}

var (
	requesterCard = cardSlot{
		channel: func(t domain.Ticket, _ domain.Queue) string { return t.SourceChannelID },
		message: func(t domain.Ticket) string { return t.MessageID },
		store: func(ctx context.Context, s *Service, t domain.Ticket, messageID string) (bool, error) {
			return s.d.Repo.SetRequesterCard(ctx, t.WorkspaceID, t.ID, messageID)
		},
	}
	queueCard = cardSlot{
		channel: func(_ domain.Ticket, q domain.Queue) string { return q.ChannelID },
		message: func(t domain.Ticket) string { return t.QueueMessageID },
		store: func(ctx context.Context, s *Service, t domain.Ticket, messageID string) (bool, error) {
			return s.d.Repo.SetQueueCard(ctx, t.WorkspaceID, t.ID, messageID)
		},
	}
)

// SyncCard makes the ticket's cards show the ticket as it is now. It is the
// handler of every helpdesk event and is safe to run again (PLT-10): a card
// that is stored is only updated, and a card that is not stored is posted
// once, its message ID kept with "only if none is stored". It runs after the
// commit, never inside the transaction that wrote the ticket. A ticket whose
// queue has no channel gets no agents' card; a form ticket gets no requester
// card.
func (s *Service) SyncCard(ctx context.Context, workspaceID, ticketID string) error {
	if s.d.Cards == nil {
		return nil
	}
	t, err := s.d.Repo.Ticket(ctx, workspaceID, ticketID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	q, err := s.d.Repo.Queue(ctx, workspaceID, t.QueueID)
	if err != nil {
		return err
	}
	payload := s.cardPayload(ctx, t, q)
	if err := s.syncSlot(ctx, requesterCard, t, q, payload); err != nil {
		return err
	}
	return s.syncSlot(ctx, queueCard, t, q, payload)
}

// syncSlot updates the card in slot when it is stored, and posts it otherwise.
func (s *Service) syncSlot(ctx context.Context, slot cardSlot, t domain.Ticket, q domain.Queue, payload CardPayload) error {
	if id := slot.message(t); id != "" {
		return s.d.Cards.Update(ctx, t.WorkspaceID, id, payload)
	}
	channelID := slot.channel(t, q)
	if channelID == "" {
		return nil
	}
	// Sent as the requester: the card reads as their request (HD-08).
	messageID, err := s.d.Cards.Post(ctx, app.Card{
		WorkspaceID: t.WorkspaceID, ChannelID: channelID, SenderID: t.RequesterID, Kind: CardKind,
		Payload: payload, FallbackText: fmt.Sprintf("Ticket %s: %s (%s)", t.Key, t.Title, t.Status),
	})
	if err != nil {
		return fmt.Errorf("helpdesk: post card: %w", err)
	}
	stored, err := slot.store(ctx, s, t, messageID)
	if err != nil {
		return err
	}
	if stored {
		return nil
	}
	// Another delivery stored its card first, so ours is a duplicate nothing
	// points to. The stored one gets the latest state.
	s.d.Log.WarnContext(ctx, "helpdesk: a card was already stored; a duplicate was posted", "ticket_id", t.ID, "message_id", messageID)
	cur, err := s.d.Repo.Ticket(ctx, t.WorkspaceID, t.ID)
	if err != nil {
		return err
	}
	if id := slot.message(cur); id != "" {
		return s.d.Cards.Update(ctx, t.WorkspaceID, id, payload)
	}
	return nil
}

func (s *Service) cardPayload(ctx context.Context, t domain.Ticket, q domain.Queue) CardPayload {
	ids := []string{t.RequesterID}
	if t.AssigneeID != "" {
		ids = append(ids, t.AssigneeID)
	}
	names := s.names(ctx, ids...)
	p := CardPayload{
		TicketID: t.ID, Key: t.Key, Title: t.Title, Status: string(t.Status), Priority: string(t.Priority),
		QueueName: q.Name, RequesterID: t.RequesterID, RequesterName: names[t.RequesterID], Version: t.UpdatedAt.UnixMilli(),
	}
	if t.AssigneeID != "" {
		id := t.AssigneeID
		p.AssigneeID, p.AssigneeName = &id, names[id]
	} else if t.Status != domain.StatusClosed {
		id := q.ID
		p.CanClaimQueueID = &id
	}
	return p
}

// names resolves display names; without a directory the map is empty.
func (s *Service) names(ctx context.Context, ids ...string) map[string]string {
	if s.d.Names == nil {
		return map[string]string{}
	}
	return s.d.Names.Names(ctx, ids)
}
