package usecase

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"
)

// Notification kinds chat produces (NOTI-06). The web renders these from the
// notification's data, so their names are part of the client contract.
const (
	kindMention     = "chat.mention"
	kindDM          = "chat.dm"
	kindThreadReply = "chat.thread_reply"
)

// notifyBodyRunes bounds the plain-text excerpt in a notification's body.
const notifyBodyRunes = 140

// NotifyInput is what chat's notification rule needs from a
// chat.message_created event. The message body and channel name are read from
// chat's own tables, so the event stays small.
type NotifyInput struct {
	WorkspaceID     string
	ChannelID       string
	MessageID       string
	Seq             int64
	SenderID        string
	ThreadRootID    string
	Mentions        []string
	MentionsChannel bool
}

// NotifyMessageCreated turns a committed message into notifications: one per
// recipient, at most, by priority mention > DM > thread reply. It never
// notifies the sender or a user who is not a member of the channel, and it
// reads only chat's tables and the display names chat already resolves
// (NOTI-06). notify stores and delivers the result.
func (s *Service) NotifyMessageCreated(ctx context.Context, in NotifyInput) ([]app.Notification, error) {
	if in.WorkspaceID == "" || in.ChannelID == "" || in.MessageID == "" || in.SenderID == "" {
		return nil, nil
	}
	m, err := s.repo.Message(ctx, in.WorkspaceID, in.MessageID)
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if m.Deleted() {
		return nil, nil
	}
	ch, err := s.repo.GetChannel(ctx, in.WorkspaceID, in.ChannelID)
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	members, err := s.repo.MemberIDs(ctx, in.WorkspaceID, in.ChannelID)
	if err != nil {
		return nil, err
	}
	member := notifySet(members)

	// kinds keeps the highest-priority kind per recipient: the first kind
	// that claims a user wins, and later ones leave it alone.
	kinds := map[string]string{}
	claim := func(userIDs []string, kind string) {
		for _, id := range userIDs {
			if id == in.SenderID || !member[id] {
				continue
			}
			if _, taken := kinds[id]; !taken {
				kinds[id] = kind
			}
		}
	}
	if len(in.Mentions) > 0 || in.MentionsChannel {
		claim(in.Mentions, kindMention)
		if in.MentionsChannel {
			claim(members, kindMention)
		}
	}
	if ch.Type == domain.TypeDirect {
		claim(members, kindDM)
	}
	if in.ThreadRootID != "" {
		participants, err := s.repo.ThreadParticipantIDs(ctx, in.WorkspaceID, in.ChannelID, in.ThreadRootID)
		if err != nil {
			return nil, err
		}
		claim(participants, kindThreadReply)
	}
	if len(kinds) == 0 {
		return []app.Notification{}, nil
	}

	names := s.dir.Names(ctx, notifiedNamesBody(m.Body, in.SenderID))
	sender := notifyName(names, in.SenderID)
	title := notifyTitle(ch, sender)
	body := notifyExcerpt(m.Body, names, notifyBodyRunes)
	link := "/chat/" + ch.ID
	key := "chat.message:" + in.MessageID

	recipients := make([]string, 0, len(kinds))
	for id := range kinds {
		recipients = append(recipients, id)
	}
	slices.Sort(recipients)
	out := make([]app.Notification, len(recipients))
	for i, id := range recipients {
		out[i] = app.Notification{
			WorkspaceID: in.WorkspaceID, UserID: id, Kind: kinds[id],
			Title: title, Body: body, Link: link, ChannelID: in.ChannelID, Data: notifyData(ch, sender, in), Key: key,
		}
	}
	return out, nil
}

// notifyData is the payload the web renders known kinds from, in the
// reader's language: sender_name and channel_name ("" for a DM) fill its
// sentence. Each notification gets its own map so a consumer cannot affect
// the others.
func notifyData(ch domain.Channel, sender string, in NotifyInput) map[string]any {
	channelName := ch.Name
	if ch.Type == domain.TypeDirect {
		channelName = ""
	}
	return map[string]any{
		"sender_name":    sender,
		"channel_name":   channelName,
		"channel_id":     ch.ID,
		"message_id":     in.MessageID,
		"seq":            in.Seq,
		"thread_root_id": in.ThreadRootID,
	}
}

// notifiedNamesBody lists the users whose names the excerpt may render: the
// sender and every mention token in the body, members or not, so a stale
// mention never shows as a raw ID.
func notifiedNamesBody(body, senderID string) []string {
	mentioned, _ := domain.ParseMentions(body)
	return append([]string{senderID}, mentioned...)
}

// notifyTitle names the sender and the place: "#channel" or "DM".
func notifyTitle(ch domain.Channel, sender string) string {
	if ch.Type == domain.TypeDirect {
		return sender + " in DM"
	}
	return sender + " in #" + ch.Name
}

// notifyName is the display name of id, or the ID when the directory does not
// know the user.
func notifyName(names map[string]string, id string) string {
	if name := names[id]; name != "" {
		return name
	}
	return id
}

// notifySet turns a list of user IDs into a membership test.
func notifySet(userIDs []string) map[string]bool {
	set := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		set[id] = true
	}
	return set
}

// notifyMentionRe mirrors domain.ParseMentions; the excerpt replaces each
// token with the user's display name.
var notifyMentionRe = regexp.MustCompile(`<@([0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})>`)

// notifyExcerpt makes a message body a one-line plain-text preview of at most
// max runes, with "<@id>" rendered as "@name" and "<!channel>" as "@channel".
// Longer text is cut and ends with an ellipsis.
func notifyExcerpt(body string, names map[string]string, max int) string {
	text := notifyMentionRe.ReplaceAllStringFunc(body, func(token string) string {
		sub := notifyMentionRe.FindStringSubmatch(token)
		id, err := uuid.Parse(sub[1])
		if err != nil {
			return token
		}
		return "@" + notifyName(names, id.String())
	})
	text = strings.ReplaceAll(text, "<!channel>", "@channel")
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	r := []rune(text)
	return string(r[:max-1]) + "…"
}
