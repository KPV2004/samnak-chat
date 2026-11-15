// Package domain holds the chat entities and rules: channel visibility,
// names, direct-message keys, message bodies, mentions and edit rights.
package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// ChannelType is who can find and read a channel (CHAT-01, CHAT-03).
type ChannelType string

const (
	TypePublic  ChannelType = "public"
	TypePrivate ChannelType = "private"
	TypeDirect  ChannelType = "dm"
)

const (
	maxNameRunes  = 80
	maxTopicRunes = 250
	maxDMOthers   = 8
)

// Channel is a conversation space inside one workspace.
type Channel struct {
	ID          string
	WorkspaceID string
	Type        ChannelType
	Name        string // empty for direct messages
	Topic       string
	DMKey       string // identifies the member set of a direct message
	CreatedBy   string
	LastSeq     int64
	ArchivedAt  *time.Time
	CreatedAt   time.Time
}

func (c Channel) Archived() bool { return c.ArchivedAt != nil }

// ValidCreateType reports whether t may be chosen when creating a channel.
// Direct messages have their own endpoint.
func ValidCreateType(t ChannelType) bool { return t == TypePublic || t == TypePrivate }

// Visible reports whether a user with role may know the channel exists.
// Public channels are open to every member except guests; everything else is
// for members of the channel only. Callers answer 404 when this is false, so
// a private channel's existence does not leak.
func Visible(c Channel, role app.Role, isMember bool) bool {
	if isMember {
		return true
	}
	return c.Type == TypePublic && role != app.RoleGuest
}

// NormalizeChannelName trims name and checks it is 1 to 80 characters
// without control characters.
func NormalizeChannelName(name string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > maxNameRunes {
		return "", fmt.Errorf("channel name must be 1 to %d characters: %w", maxNameRunes, app.ErrInvalid)
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("channel name must not contain control characters: %w", app.ErrInvalid)
	}
	return name, nil
}

// NormalizeTopic trims topic and checks it is at most 250 characters.
func NormalizeTopic(topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if utf8.RuneCountInString(topic) > maxTopicRunes {
		return "", fmt.Errorf("topic must be at most %d characters: %w", maxTopicRunes, app.ErrInvalid)
	}
	if strings.ContainsRune(topic, 0) {
		return "", fmt.Errorf("topic must not contain NUL: %w", app.ErrInvalid)
	}
	return topic, nil
}

// ParseIDs checks that every entry is a UUID and returns them in canonical
// lower-case form without duplicates, in first-seen order.
func ParseIDs(ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	for _, s := range ids {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("%q is not a valid user ID: %w", s, app.ErrInvalid)
		}
		c := u.String()
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out, nil
}

// DirectKey returns the key shared by every direct message with exactly the
// member set {self} plus others, and that sorted member set. others must hold
// 1 to 8 users besides self (CHAT-03).
func DirectKey(self string, others []string) (key string, members []string, err error) {
	all, err := ParseIDs(append([]string{self}, others...))
	if err != nil {
		return "", nil, err
	}
	if len(all) < 2 {
		return "", nil, fmt.Errorf("a direct message needs at least one other user: %w", app.ErrInvalid)
	}
	if len(all)-1 > maxDMOthers {
		return "", nil, fmt.Errorf("a direct message holds at most %d other users: %w", maxDMOthers, app.ErrInvalid)
	}
	slices.Sort(all)
	return strings.Join(all, ","), all, nil
}

// Actions of the chat module for the shared permission check (WSP-07). A rule
// that also depends on the resource, such as "the author", is checked in the
// use case after Can has passed.
const (
	// ActionChannelCreate lets a user create channels. Guests cannot.
	ActionChannelCreate = "chat.channel.create"
	// ActionChannelManage lets a user edit or archive any channel; the
	// creator of a channel may always do so.
	ActionChannelManage = "chat.channel.manage"
	// ActionMessageModerate lets a user delete anyone's message (CHAT-10).
	ActionMessageModerate = "chat.message.moderate"
)
