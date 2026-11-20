// Package usecase holds the chat flows and the ports they need.
package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// ChannelView is a channel as one user sees it.
type ChannelView struct {
	domain.Channel
	IsMember     bool
	PinnedAt     *time.Time // when the user pinned the channel; nil when not pinned
	LastReadSeq  int64
	MemberCount  int64
	UnreadCount  int64
	MentionCount int64
	DMUserIDs    []string        // members of a direct message, else empty
	LastMessage  *domain.Message // the newest message, deleted ones included; nil when there is none
}

// MessageView is a message with its thread size, files and reactions. A
// deleted message has neither files nor reactions.
type MessageView struct {
	domain.Message
	ReplyCount  int64
	Attachments []domain.Attachment
	Reactions   []ReactionGroup
}

// ReactionGroup is the people who used one emoji on a message, oldest first.
// Groups are ordered by when the emoji first appeared.
type ReactionGroup struct {
	Emoji   string
	UserIDs []string
}

// Member is a channel member.
type Member struct {
	UserID      string
	DisplayName string // empty when the user directory is unavailable
	AvatarURL   string // empty when the user has no picture or the directory is unavailable
	JoinedAt    time.Time
}

// ViewMode selects which channels Repository.ChannelViews returns.
type ViewMode string

const (
	ViewOne    ViewMode = "one"    // the channel with the given ID
	ViewJoined ViewMode = "joined" // channels the user is a member of
	ViewPublic ViewMode = "public" // unarchived public channels
)

// Repository is the storage the use cases need. Every method takes the
// workspace ID, which callers read from the Principal. Methods return an
// error wrapping app.ErrNotFound for a missing row and app.ErrConflict for a
// uniqueness violation.
type Repository interface {
	InsertChannel(ctx context.Context, c domain.Channel) error
	// DirectChannelID finds the direct message with the given member key.
	DirectChannelID(ctx context.Context, workspaceID, dmKey string) (string, error)
	GetChannel(ctx context.Context, workspaceID, channelID string) (domain.Channel, error)
	UpdateChannel(ctx context.Context, c domain.Channel) error
	// NextSeq increments the channel's last_seq and returns it, holding the
	// channel row lock until the transaction ends. It fails with ErrNotFound
	// when the channel is missing or archived.
	NextSeq(ctx context.Context, workspaceID, channelID string) (int64, error)
	// ChannelViews returns channels for userID. channelID is used by ViewOne.
	ChannelViews(ctx context.Context, workspaceID, userID string, mode ViewMode, channelID string) ([]ChannelView, error)

	// AddMembers returns the users that were not members yet.
	AddMembers(ctx context.Context, workspaceID, channelID string, userIDs []string, at time.Time) ([]string, error)
	// RemoveMember reports whether the user was a member.
	RemoveMember(ctx context.Context, workspaceID, channelID, userID string) (bool, error)
	// RemoveUserEverywhere removes the user from every channel of the
	// workspace and returns those channel IDs.
	RemoveUserEverywhere(ctx context.Context, workspaceID, userID string) ([]string, error)
	// LockPins locks every membership the user has in the workspace until the
	// transaction ends, and returns the IDs of the channels they have pinned.
	LockPins(ctx context.Context, workspaceID, userID string) ([]string, error)
	// SetPin pins the channel for the user at pinnedAt, or unpins it when
	// pinnedAt is nil. It reports whether the user is in the channel.
	SetPin(ctx context.Context, workspaceID, channelID, userID string, pinnedAt *time.Time) (bool, error)
	MemberIDs(ctx context.Context, workspaceID, channelID string) ([]string, error)
	Members(ctx context.Context, workspaceID, channelID string) ([]Member, error)
	// FilterMembers returns the userIDs that are members of the channel.
	FilterMembers(ctx context.Context, workspaceID, channelID string, userIDs []string) ([]string, error)
	// ThreadParticipantIDs returns the users who follow a thread: the author
	// of the root message and everyone who replied under it (NOTI-06). It
	// does not filter to channel members; the caller does.
	ThreadParticipantIDs(ctx context.Context, workspaceID, channelID, rootMessageID string) ([]string, error)
	// AdvanceRead moves the user's read position forward to seq, capped at
	// the channel's last_seq. advanced is false when nothing changed.
	AdvanceRead(ctx context.Context, workspaceID, channelID, userID string, seq int64) (lastRead int64, advanced bool, err error)

	// InsertMessage fails with ErrConflict on a duplicate (channel, seq) or
	// (sender, client_msg_id).
	InsertMessage(ctx context.Context, m domain.Message) error
	MessageByClientID(ctx context.Context, workspaceID, senderID, clientMsgID string) (MessageView, error)
	Message(ctx context.Context, workspaceID, messageID string) (MessageView, error)
	// MessagesAfter returns up to limit messages with seq > afterSeq,
	// ascending. threadRootID filters to replies of one root when not empty.
	MessagesAfter(ctx context.Context, workspaceID, channelID string, afterSeq int64, threadRootID string, limit int) ([]MessageView, error)
	// MessagesBefore returns up to limit messages with seq < beforeSeq
	// (0 means the newest), descending.
	MessagesBefore(ctx context.Context, workspaceID, channelID string, beforeSeq int64, threadRootID string, limit int) ([]MessageView, error)
	UpdateMessageBody(ctx context.Context, workspaceID, messageID, body string, mentions []string, mentionsChannel bool, at time.Time) error
	SoftDeleteMessage(ctx context.Context, workspaceID, messageID string, at time.Time) error

	// AddReaction reports whether the reaction is new.
	AddReaction(ctx context.Context, workspaceID, messageID, userID, emoji string, at time.Time) (added bool, err error)
	// RemoveReaction reports whether the reaction existed.
	RemoveReaction(ctx context.Context, workspaceID, messageID, userID, emoji string) (removed bool, err error)

	InsertAttachment(ctx context.Context, a domain.Attachment) error
	Attachment(ctx context.Context, workspaceID, attachmentID string) (domain.Attachment, error)
	// AttachmentsByIDs returns the files that exist, in upload order.
	AttachmentsByIDs(ctx context.Context, workspaceID string, attachmentIDs []string) ([]domain.Attachment, error)
	// AttachToMessage links files the uploader put in the channel and that no
	// message uses yet. It fails with ErrConflict when any file does not
	// qualify, so the surrounding transaction rolls back.
	AttachToMessage(ctx context.Context, workspaceID, channelID, uploaderID, messageID string, attachmentIDs []string) error

	// UpdateMessagePayload replaces the payload of a card message.
	UpdateMessagePayload(ctx context.Context, workspaceID, messageID string, payload json.RawMessage) error
	// MemberWorkspace returns the workspace of a channel the user is a member
	// of, or ErrNotFound.
	MemberWorkspace(ctx context.Context, channelID, userID string) (workspaceID string, err error)
}

// Workspace answers membership questions through the workspace module.
type Workspace interface {
	// IsActiveMember reports whether the user is an active member of the
	// workspace. A false answer is not an error.
	IsActiveMember(ctx context.Context, workspaceID, userID string) (bool, error)
	// MemberRole returns the user's role. active is false, with no error,
	// when the user is not an active member.
	MemberRole(ctx context.Context, workspaceID, userID string) (role app.Role, active bool, err error)
}

// Permissions is the shared permission check (WSP-07).
type Permissions interface {
	Can(p app.Principal, action string) bool
}

// Profile is what the use cases show about someone: a name and a picture.
type Profile struct {
	DisplayName string
	AvatarURL   string
}

// Directory looks up display names and pictures. Unknown users are absent.
type Directory interface {
	Names(ctx context.Context, ids []string) map[string]string
	Profiles(ctx context.Context, ids []string) map[string]Profile
}

// Events announces what happened. Methods are called after the transaction
// commits, are best effort, and never fail the request.
type Events interface {
	MessageCreated(ctx context.Context, ch domain.Channel, m MessageView, recipients []string)
	MessageUpdated(ctx context.Context, m MessageView, recipients []string)
	MessageDeleted(ctx context.Context, m MessageView, recipients []string)
	// CardUpdated tells recipients a card's payload changed (m has the new one).
	CardUpdated(ctx context.Context, m MessageView, recipients []string)
	// Typing tells recipients that userID is typing in a channel.
	Typing(ctx context.Context, workspaceID, channelID, userID string, recipients []string)
	// ChannelRead goes to the reader's own devices only.
	ChannelRead(ctx context.Context, v ChannelView, userID string)
	// ChannelPinned goes to the user's own devices only. v.PinnedAt is the new
	// pin, nil when it was removed.
	ChannelPinned(ctx context.Context, v ChannelView, userID string)
	// ChannelUpdated tells recipients that a channel was created, changed or
	// its member list changed.
	ChannelUpdated(ctx context.Context, v ChannelView, recipients []string)
}
