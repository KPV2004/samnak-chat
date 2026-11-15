package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// KindText is the kind of an ordinary message; other kinds are cards.
const KindText = "text"

// MaxBodyRunes is the longest message body (CHAT-04).
const MaxBodyRunes = 4000

// Message is one entry in a channel. A deleted message keeps its row and seq,
// with an empty body and DeletedAt set (CHAT-10).
type Message struct {
	ID              string
	WorkspaceID     string
	ChannelID       string
	Seq             int64
	SenderID        string
	ClientMsgID     string
	Kind            string
	Body            string
	Payload         json.RawMessage // card payload, nil for text
	FallbackText    string
	ThreadRootID    string // empty for a top-level message
	Mentions        []string
	MentionsChannel bool
	EditedAt        *time.Time
	DeletedAt       *time.Time
	CreatedAt       time.Time
}

func (m Message) Deleted() bool { return m.DeletedAt != nil }

// ValidateBody checks a message body: markdown text of 1 to 4,000 characters.
func ValidateBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("message body must not be empty: %w", app.ErrInvalid)
	}
	if utf8.RuneCountInString(body) > MaxBodyRunes {
		return fmt.Errorf("message body must be at most %d characters: %w", MaxBodyRunes, app.ErrInvalid)
	}
	if strings.ContainsRune(body, 0) {
		return fmt.Errorf("message body must not contain NUL: %w", app.ErrInvalid)
	}
	return nil
}

// ValidateContent checks a new message: a body of 1 to 4,000 characters, or,
// when the message has attachments, a body that may be blank.
func ValidateContent(body string, attachments int) error {
	if attachments > 0 && strings.TrimSpace(body) == "" && utf8.RuneCountInString(body) <= MaxBodyRunes && !strings.ContainsRune(body, 0) {
		return nil
	}
	return ValidateBody(body)
}

var (
	mentionUserRe    = regexp.MustCompile(`<@([0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})>`)
	mentionChannelRe = regexp.MustCompile(`<!channel>`)
)

// ParseMentions reads the mention tokens of a body: "<@user_id>" and
// "<!channel>" (CHAT-13). User IDs come back canonical and without
// duplicates. The caller keeps only the users who are in the channel.
func ParseMentions(body string) (userIDs []string, channel bool) {
	userIDs = []string{}
	for _, m := range mentionUserRe.FindAllStringSubmatch(body, -1) {
		id, err := uuid.Parse(m[1])
		if err != nil {
			continue
		}
		s := id.String()
		seen := false
		for _, have := range userIDs {
			if have == s {
				seen = true
				break
			}
		}
		if !seen {
			userIDs = append(userIDs, s)
		}
	}
	return userIDs, mentionChannelRe.MatchString(body)
}

// CanEdit checks that userID may edit the message: only the author, only text
// messages, never a deleted one.
func (m Message) CanEdit(userID string) error {
	if m.Deleted() {
		return fmt.Errorf("message: %w", app.ErrNotFound)
	}
	if m.SenderID != userID {
		return fmt.Errorf("only the author can edit a message: %w", app.ErrForbidden)
	}
	if m.Kind != KindText {
		return fmt.Errorf("only text messages can be edited: %w", app.ErrInvalid)
	}
	return nil
}

// CanDelete checks that userID may delete the message: the author, or a
// moderator (owner or admin) for anyone's.
func (m Message) CanDelete(userID string, moderator bool) error {
	if m.Deleted() {
		return fmt.Errorf("message: %w", app.ErrNotFound)
	}
	if m.SenderID != userID && !moderator {
		return fmt.Errorf("only the author or an admin can delete a message: %w", app.ErrForbidden)
	}
	return nil
}
