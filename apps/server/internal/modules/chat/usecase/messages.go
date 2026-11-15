package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// ListQuery selects a page of messages. Set at most one of AfterSeq and
// BeforeSeq. With neither, the newest messages are returned.
type ListQuery struct {
	AfterSeq     *int64 // catch-up: messages with a larger seq, ascending
	BeforeSeq    *int64 // history: messages with a smaller seq
	Limit        int
	ThreadRootID string // only replies to this message
}

// Page is a page of messages in ascending seq order.
type Page struct {
	Items   []MessageView
	HasMore bool
}

// ListMessages returns messages of a channel the caller may see (CHAT-08,
// CHAT-09). Replies are included, each with its thread_root_id.
func (s *Service) ListMessages(ctx context.Context, p app.Principal, channelID string, q ListQuery) (Page, error) {
	if q.AfterSeq != nil && q.BeforeSeq != nil {
		return Page{}, fmt.Errorf("use either after_seq or before_seq: %w", app.ErrInvalid)
	}
	if (q.AfterSeq != nil && *q.AfterSeq < 0) || (q.BeforeSeq != nil && *q.BeforeSeq < 0) {
		return Page{}, fmt.Errorf("seq must not be negative: %w", app.ErrInvalid)
	}
	if q.ThreadRootID != "" {
		if _, err := uuid.Parse(q.ThreadRootID); err != nil {
			return Page{}, fmt.Errorf("thread_root_id is not a valid ID: %w", app.ErrInvalid)
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	if _, err := s.channel(ctx, p, channelID); err != nil {
		return Page{}, err
	}
	var (
		items []MessageView
		err   error
	)
	if q.AfterSeq != nil {
		items, err = s.repo.MessagesAfter(ctx, p.WorkspaceID, channelID, *q.AfterSeq, q.ThreadRootID, limit+1)
	} else {
		var before int64
		if q.BeforeSeq != nil {
			before = *q.BeforeSeq
		}
		items, err = s.repo.MessagesBefore(ctx, p.WorkspaceID, channelID, before, q.ThreadRootID, limit+1)
	}
	if err != nil {
		return Page{}, err
	}
	page := Page{HasMore: len(items) > limit}
	if page.HasMore {
		items = items[:limit]
	}
	if q.AfterSeq == nil {
		// History is read newest first from the database; clients want
		// ascending order within the page.
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	page.Items = items
	return page, nil
}

// SendInput is a new text message.
type SendInput struct {
	ChannelID    string
	ClientMsgID  string
	Body         string
	ThreadRootID string
}

// SendMessage stores a message with the next seq of the channel. Sending the
// same client_msg_id again returns the stored message with created=false and
// publishes nothing (CHAT-05, CHAT-06).
func (s *Service) SendMessage(ctx context.Context, p app.Principal, in SendInput) (v MessageView, created bool, err error) {
	clientID, err := uuid.Parse(in.ClientMsgID)
	if err != nil {
		return MessageView{}, false, fmt.Errorf("client_msg_id must be a UUID: %w", app.ErrInvalid)
	}
	in.ClientMsgID = clientID.String()
	if err := domain.ValidateBody(in.Body); err != nil {
		return MessageView{}, false, err
	}
	ch, err := s.memberChannel(ctx, p, in.ChannelID)
	if err != nil {
		return MessageView{}, false, err
	}
	// A retry finds its message here without taking the channel lock.
	if prev, ok, err := s.previous(ctx, p, ch.ID, in.ClientMsgID); err != nil || ok {
		return prev, false, err
	}
	if ch.Archived() {
		return MessageView{}, false, fmt.Errorf("the channel is archived: %w", app.ErrConflict)
	}
	root, err := s.threadRoot(ctx, p, ch.ID, in.ThreadRootID)
	if err != nil {
		return MessageView{}, false, err
	}
	mentioned, mentionsChannel := domain.ParseMentions(in.Body)
	mentions, err := s.repo.FilterMembers(ctx, p.WorkspaceID, ch.ID, mentioned)
	if err != nil {
		return MessageView{}, false, err
	}

	msg := domain.Message{
		ID: newID(), WorkspaceID: p.WorkspaceID, ChannelID: ch.ID, SenderID: p.UserID, ClientMsgID: in.ClientMsgID,
		Kind: domain.KindText, Body: in.Body, Mentions: mentions, MentionsChannel: mentionsChannel,
		CreatedAt: s.clock(),
	}
	if root != nil {
		msg.ThreadRootID = root.ID
	}
	var existing *MessageView
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		// The channel row lock taken here makes senders to one channel run
		// one at a time, so seq is dense: a rollback undoes the increment.
		seq, err := s.repo.NextSeq(ctx, p.WorkspaceID, ch.ID)
		if errors.Is(err, app.ErrNotFound) {
			return fmt.Errorf("the channel is archived: %w", app.ErrConflict)
		}
		if err != nil {
			return err
		}
		// Two sends with one client_msg_id that raced past the check above
		// meet here, one after the other.
		if prev, ok, err := s.previous(ctx, p, ch.ID, in.ClientMsgID); err != nil {
			return err
		} else if ok {
			existing = &prev
			return nil
		}
		msg.Seq = seq
		return s.repo.InsertMessage(ctx, msg)
	})
	if err != nil {
		return MessageView{}, false, err
	}
	if existing != nil {
		return *existing, false, nil
	}

	v = MessageView{Message: msg}
	if recipients, err := s.repo.MemberIDs(ctx, p.WorkspaceID, ch.ID); err != nil {
		s.log.WarnContext(ctx, "chat: list recipients", "channel_id", ch.ID, "error", err)
	} else {
		s.ev.MessageCreated(ctx, ch.Channel, v, recipients)
		if root != nil {
			s.refreshRoot(ctx, p.WorkspaceID, root.ID, recipients)
		}
	}
	return v, true, nil
}

// previous returns the message p already sent with clientMsgID. Reusing an ID
// for another channel is a conflict, not a retry.
func (s *Service) previous(ctx context.Context, p app.Principal, channelID, clientMsgID string) (MessageView, bool, error) {
	prev, err := s.repo.MessageByClientID(ctx, p.WorkspaceID, p.UserID, clientMsgID)
	if errors.Is(err, app.ErrNotFound) {
		return MessageView{}, false, nil
	}
	if err != nil {
		return MessageView{}, false, err
	}
	if prev.ChannelID != channelID {
		return MessageView{}, false, fmt.Errorf("client_msg_id was already used in another channel: %w", app.ErrConflict)
	}
	return prev, true, nil
}

// threadRoot checks that rootID, when given, is a top-level message of the
// channel. Threads are one level deep (CHAT-11).
func (s *Service) threadRoot(ctx context.Context, p app.Principal, channelID, rootID string) (*MessageView, error) {
	if rootID == "" {
		return nil, nil
	}
	root, err := s.repo.Message(ctx, p.WorkspaceID, rootID)
	if err != nil || root.ChannelID != channelID || root.ThreadRootID != "" || root.Deleted() {
		if err != nil && !errors.Is(err, app.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("thread_root_id must be a top-level message of this channel: %w", app.ErrInvalid)
	}
	return &root, nil
}

// refreshRoot tells clients a thread root's reply_count changed.
func (s *Service) refreshRoot(ctx context.Context, workspaceID, rootID string, recipients []string) {
	root, err := s.repo.Message(ctx, workspaceID, rootID)
	if err != nil {
		s.log.WarnContext(ctx, "chat: reload thread root", "message_id", rootID, "error", err)
		return
	}
	s.ev.MessageUpdated(ctx, root, recipients)
}

// message loads a message and the channel it is in, as p sees them.
func (s *Service) message(ctx context.Context, p app.Principal, messageID string) (MessageView, ChannelView, error) {
	m, err := s.repo.Message(ctx, p.WorkspaceID, messageID)
	if err != nil {
		return MessageView{}, ChannelView{}, err
	}
	ch, err := s.channel(ctx, p, m.ChannelID)
	if err != nil {
		return MessageView{}, ChannelView{}, err
	}
	return m, ch, nil
}

// EditMessage replaces the body of the caller's own message (CHAT-10).
func (s *Service) EditMessage(ctx context.Context, p app.Principal, messageID, body string) (MessageView, error) {
	if err := domain.ValidateBody(body); err != nil {
		return MessageView{}, err
	}
	m, ch, err := s.message(ctx, p, messageID)
	if err != nil {
		return MessageView{}, err
	}
	if err := m.CanEdit(p.UserID); err != nil {
		return MessageView{}, err
	}
	if !ch.IsMember {
		return MessageView{}, fmt.Errorf("join the channel first: %w", app.ErrForbidden)
	}
	mentioned, mentionsChannel := domain.ParseMentions(body)
	mentions, err := s.repo.FilterMembers(ctx, p.WorkspaceID, ch.ID, mentioned)
	if err != nil {
		return MessageView{}, err
	}
	if err := s.repo.UpdateMessageBody(ctx, p.WorkspaceID, m.ID, body, mentions, mentionsChannel, s.clock()); err != nil {
		return MessageView{}, err
	}
	updated, err := s.repo.Message(ctx, p.WorkspaceID, m.ID)
	if err != nil {
		return MessageView{}, err
	}
	s.deliver(ctx, p.WorkspaceID, ch.ID, func(r []string) { s.ev.MessageUpdated(ctx, updated, r) })
	return updated, nil
}

// DeleteMessage soft-deletes a message: the author may delete their own, and
// owners and admins anyone's (CHAT-10).
func (s *Service) DeleteMessage(ctx context.Context, p app.Principal, messageID string) error {
	m, ch, err := s.message(ctx, p, messageID)
	if err != nil {
		return err
	}
	if err := m.CanDelete(p.UserID, s.perms.Can(p, domain.ActionMessageModerate)); err != nil {
		return err
	}
	if err := s.repo.SoftDeleteMessage(ctx, p.WorkspaceID, m.ID, s.clock()); err != nil {
		return err
	}
	deleted, err := s.repo.Message(ctx, p.WorkspaceID, m.ID)
	if err != nil {
		return err
	}
	s.deliver(ctx, p.WorkspaceID, ch.ID, func(r []string) {
		s.ev.MessageDeleted(ctx, deleted, r)
		if m.ThreadRootID != "" {
			s.refreshRoot(ctx, p.WorkspaceID, m.ThreadRootID, r)
		}
	})
	return nil
}

// deliver runs send with the members of the channel, after the commit.
func (s *Service) deliver(ctx context.Context, workspaceID, channelID string, send func(recipients []string)) {
	recipients, err := s.repo.MemberIDs(ctx, workspaceID, channelID)
	if err != nil {
		s.log.WarnContext(ctx, "chat: list recipients", "channel_id", channelID, "error", err)
		return
	}
	send(recipients)
}

// MarkRead moves the caller's read position forward to seq (CHAT-12). A
// smaller seq is ignored, and seq is capped at the channel's last seq.
func (s *Service) MarkRead(ctx context.Context, p app.Principal, channelID string, seq int64) error {
	if seq < 0 {
		return fmt.Errorf("seq must not be negative: %w", app.ErrInvalid)
	}
	if _, err := s.memberChannel(ctx, p, channelID); err != nil {
		return err
	}
	_, advanced, err := s.repo.AdvanceRead(ctx, p.WorkspaceID, channelID, p.UserID, seq)
	if err != nil || !advanced {
		return err
	}
	v, err := s.channel(ctx, p, channelID)
	if err != nil {
		return err
	}
	s.ev.ChannelRead(ctx, v, p.UserID)
	return nil
}
