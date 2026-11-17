package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/domain"
)

// CardKind is the kind of the task card (TASK-03).
const CardKind = "task.item"

// CardPayload is the payload of a task.item card. It carries what the web
// needs to draw the card without another request. Version is the task's
// updated_at in Unix milliseconds, so a late card.updated never overwrites a
// newer one on the client.
type CardPayload struct {
	TaskID       string     `json:"task_id"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	AssigneeID   *string    `json:"assignee_id"`
	AssigneeName string     `json:"assignee_name"`
	DueAt        *time.Time `json:"due_at"`
	Version      int64      `json:"version"`
}

func (s *Service) cardPayload(ctx context.Context, t domain.Task) CardPayload {
	p := CardPayload{
		TaskID: t.ID, Title: t.Title, Status: string(t.Status), Priority: string(t.Priority),
		DueAt: t.DueAt, Version: t.UpdatedAt.UnixMilli(),
	}
	if t.AssigneeID != "" {
		id := t.AssigneeID
		p.AssigneeID = &id
		p.AssigneeName = s.names(ctx, id)[id]
	}
	return p
}

// names resolves display names; unknown users and a missing directory give an
// empty map.
func (s *Service) names(ctx context.Context, ids ...string) map[string]string {
	if s.d.Directory == nil {
		return map[string]string{}
	}
	return s.d.Directory.Names(ctx, ids)
}

// SyncCard makes the task's card show the task as it is now. It is the
// handler of every task event and is safe to run again (PLT-10): without a
// card it posts one in the task's channel as its creator and stores the
// message ID with "only if none is stored"; with a card it only replaces the
// payload. A delivery that posted while another stored its card first
// refreshes the stored one. It must run after the commit, never inside the
// transaction that wrote the task.
func (s *Service) SyncCard(ctx context.Context, workspaceID, taskID string) error {
	if s.d.Cards == nil {
		return nil
	}
	t, err := s.d.Repo.Get(ctx, workspaceID, taskID)
	if err != nil {
		if isNotFound(err) {
			return nil // deleted since the event was published
		}
		return err
	}
	payload := s.cardPayload(ctx, t)
	if t.MessageID != "" {
		return s.d.Cards.Update(ctx, workspaceID, t.MessageID, payload)
	}

	msgID, err := s.d.Cards.Post(ctx, app.Card{
		WorkspaceID: t.WorkspaceID, ChannelID: t.ChannelID, SenderID: t.CreatorID,
		Kind: CardKind, Payload: payload, FallbackText: "Task: " + t.Title,
	})
	if err != nil {
		return fmt.Errorf("task: post card: %w", err)
	}
	won, err := s.d.Repo.SetMessage(ctx, workspaceID, taskID, msgID)
	if err != nil {
		return err
	}
	if won {
		return nil
	}
	// Another delivery stored its card first. Ours stays in the channel as a
	// stray message (chat cannot delete a card for us); the stored card gets
	// the latest state.
	s.d.Log.WarnContext(ctx, "task: card posted twice", "task_id", taskID, "message_id", msgID)
	t, err = s.d.Repo.Get(ctx, workspaceID, taskID)
	if err != nil {
		return err
	}
	return s.d.Cards.Update(ctx, workspaceID, t.MessageID, s.cardPayload(ctx, t))
}
