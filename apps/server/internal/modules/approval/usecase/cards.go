package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/approval/domain"
)

// CardKind is the chat card kind this module owns (PLT-11).
const CardKind = "approval.request"

// CardPayload is the payload of an approval.request card. It carries what the
// web needs to draw the card without another request. Version is the
// request's updated_at in Unix milliseconds, so a late card.updated never
// replaces a newer one on the client.
type CardPayload struct {
	RequestID     string         `json:"request_id"`
	Type          string         `json:"type"`
	Status        string         `json:"status"`
	Summary       string         `json:"summary"`
	Fields        map[string]any `json:"fields"`
	RequesterID   string         `json:"requester_id"`
	RequesterName string         `json:"requester_name"`
	ApproverID    string         `json:"approver_id"`
	ApproverName  string         `json:"approver_name"`
	Reason        string         `json:"reason"`
	DecidedAt     *time.Time     `json:"decided_at"`
	Version       int64          `json:"version"`
}

func isNotFound(err error) bool { return errors.Is(err, app.ErrNotFound) }

func (s *Service) names(ctx context.Context, ids ...string) map[string]string {
	if s.Names == nil {
		return map[string]string{}
	}
	return s.Names.Names(ctx, ids)
}

func (s *Service) cardPayload(ctx context.Context, r domain.Request) CardPayload {
	fields := map[string]any{}
	if err := json.Unmarshal(r.Payload, &fields); err != nil {
		// The column only holds what ParsePayload produced.
		s.Log.WarnContext(ctx, "approval: stored payload does not decode", "request_id", r.ID, "error", err)
	}
	names := s.names(ctx, r.RequesterID, r.ApproverID)
	return CardPayload{
		RequestID: r.ID, Type: string(r.Type), Status: string(r.Status), Summary: r.Summary(), Fields: fields,
		RequesterID: r.RequesterID, RequesterName: names[r.RequesterID], ApproverID: r.ApproverID, ApproverName: names[r.ApproverID],
		Reason: r.Reason, DecidedAt: r.DecidedAt, Version: r.Version(),
	}
}

// SyncCard makes the chat card show the request as it is now: it posts the
// card in the requester-approver DM when there is none, and updates it
// otherwise. Every approval event calls it, from the event handler after the
// commit, and it is idempotent: a redelivered event finds the card stored and
// only rewrites the payload (PLT-10). Reading the row, not the event, means a
// late event never shows an old status.
func (s *Service) SyncCard(ctx context.Context, workspaceID, requestID string) error {
	if s.Cards == nil {
		return nil
	}
	req, err := s.Repo.Request(ctx, workspaceID, requestID)
	if err != nil {
		return err
	}
	payload := s.cardPayload(ctx, req)
	if req.MessageID != "" {
		return s.Cards.Update(ctx, workspaceID, req.MessageID, payload)
	}
	fallback := fmt.Sprintf("Approval request: %s (%s)", payload.Summary, payload.Status)
	channelID, messageID, err := s.Cards.Post(ctx, workspaceID, req.RequesterID, req.ApproverID, payload, fallback)
	if err != nil {
		return fmt.Errorf("approval: post card: %w", err)
	}
	changed, err := s.Repo.SetCard(ctx, workspaceID, req.ID, channelID, messageID)
	if err != nil {
		return err
	}
	if changed {
		return nil
	}
	// Another delivery stored its card first, so ours is a duplicate that
	// nothing points to. Keep the stored one up to date.
	s.Log.WarnContext(ctx, "approval: a card was already stored; a duplicate was posted", "request_id", req.ID, "message_id", messageID)
	cur, err := s.Repo.Request(ctx, workspaceID, requestID)
	if err != nil {
		return err
	}
	if cur.MessageID == "" {
		return nil
	}
	return s.Cards.Update(ctx, workspaceID, cur.MessageID, payload)
}
