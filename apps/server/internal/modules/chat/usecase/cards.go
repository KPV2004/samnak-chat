package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// maxPayloadBytes bounds a card payload as stored.
const maxPayloadBytes = 64 << 10

// nulEscape is how encoding/json writes a NUL inside a string; Postgres jsonb
// rejects it.
var nulEscape = []byte(`\u0000`)

// EncodePayload marshals a card payload and checks it fits the database: a
// JSON value that is not null, at most 64 KiB, with no NUL characters.
func EncodePayload(payload any) (json.RawMessage, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("card payload cannot be encoded as JSON: %w", app.ErrInvalid)
	}
	if string(b) == "null" {
		return nil, fmt.Errorf("card payload must not be empty: %w", app.ErrInvalid)
	}
	if len(b) > maxPayloadBytes {
		return nil, fmt.Errorf("card payload must be at most %d KiB: %w", maxPayloadBytes>>10, app.ErrInvalid)
	}
	if bytes.Contains(b, nulEscape) {
		return nil, fmt.Errorf("card payload must not contain NUL: %w", app.ErrInvalid)
	}
	return b, nil
}

// PostCard stores a card message from another module and sends message.created
// (CHAT-15). The sender must be an active member of the workspace but need not
// be in the channel: cards are often system messages shown as sent by a person.
func (s *Service) PostCard(ctx context.Context, c app.Card, payload json.RawMessage) (MessageView, error) {
	for what, id := range map[string]string{"workspace": c.WorkspaceID, "channel": c.ChannelID, "sender": c.SenderID} {
		if _, err := uuid.Parse(id); err != nil {
			return MessageView{}, fmt.Errorf("card %s ID is not a valid ID: %w", what, app.ErrInvalid)
		}
	}
	if strings.TrimSpace(c.FallbackText) == "" || utf8.RuneCountInString(c.FallbackText) > domain.MaxBodyRunes || strings.ContainsRune(c.FallbackText, 0) {
		return MessageView{}, fmt.Errorf("card fallback_text must be 1 to %d characters: %w", domain.MaxBodyRunes, app.ErrInvalid)
	}
	active, err := s.ws.IsActiveMember(ctx, c.WorkspaceID, c.SenderID)
	if err != nil {
		return MessageView{}, err
	}
	if !active {
		return MessageView{}, fmt.Errorf("card sender is not a member of this workspace: %w", app.ErrInvalid)
	}
	ch, err := s.repo.GetChannel(ctx, c.WorkspaceID, c.ChannelID)
	if err != nil {
		return MessageView{}, err
	}
	if ch.Archived() {
		return MessageView{}, fmt.Errorf("the channel is archived: %w", app.ErrConflict)
	}
	root, err := s.threadRoot(ctx, app.Principal{WorkspaceID: c.WorkspaceID}, ch.ID, c.ThreadRootID)
	if err != nil {
		return MessageView{}, err
	}
	msg := domain.Message{
		ID: newID(), WorkspaceID: c.WorkspaceID, ChannelID: ch.ID, SenderID: c.SenderID, ClientMsgID: uuid.NewString(),
		Kind: c.Kind, Payload: payload, FallbackText: c.FallbackText, CreatedAt: s.clock(),
	}
	if root != nil {
		msg.ThreadRootID = root.ID
	}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		seq, err := s.repo.NextSeq(ctx, c.WorkspaceID, ch.ID)
		if err != nil {
			return err
		}
		msg.Seq = seq
		return s.repo.InsertMessage(ctx, msg)
	})
	if err != nil {
		return MessageView{}, err
	}
	v := MessageView{Message: msg}
	s.announceCreated(ctx, ch, v, root)
	return v, nil
}

// UpdateCard replaces the payload of a card message and sends card.updated to
// the channel. registered tells whether a card kind may be updated; any other
// message, including text, is rejected.
func (s *Service) UpdateCard(ctx context.Context, workspaceID, messageID string, payload json.RawMessage, registered func(kind string) bool) error {
	if _, err := uuid.Parse(workspaceID); err != nil {
		return fmt.Errorf("workspace ID is not a valid ID: %w", app.ErrInvalid)
	}
	m, err := s.repo.Message(ctx, workspaceID, messageID)
	if err != nil {
		return err
	}
	if m.Deleted() {
		return fmt.Errorf("card: %w", app.ErrNotFound)
	}
	if m.Kind == domain.KindText || !registered(m.Kind) {
		return fmt.Errorf("message is not a card of a registered kind: %w", app.ErrInvalid)
	}
	if err := s.repo.UpdateMessagePayload(ctx, workspaceID, m.ID, payload); err != nil {
		return err
	}
	m.Payload = payload
	s.deliver(ctx, workspaceID, m.ChannelID, func(r []string) { s.ev.CardUpdated(ctx, m, r) })
	return nil
}
