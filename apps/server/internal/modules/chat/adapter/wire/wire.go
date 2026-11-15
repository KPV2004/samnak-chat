// Package wire holds the JSON shapes of chat that both REST responses and
// WebSocket events use, so a client decodes one Message and one Channel.
package wire

import (
	"encoding/json"
	"time"

	"github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/usecase"
)

// Channel is a channel as one user sees it.
type Channel struct {
	ID           string     `json:"id"`
	Type         string     `json:"type" enum:"public,private,dm"`
	Name         string     `json:"name" doc:"Empty for direct messages"`
	Topic        string     `json:"topic"`
	LastSeq      int64      `json:"last_seq"`
	LastReadSeq  int64      `json:"last_read_seq" doc:"The caller's read position; 0 when not a member"`
	UnreadCount  int64      `json:"unread_count"`
	MentionCount int64      `json:"mention_count"`
	MemberCount  int64      `json:"member_count"`
	DMUserIDs    []string   `json:"dm_user_ids" nullable:"false" doc:"Members of a direct message, empty otherwise"`
	ArchivedAt   *time.Time `json:"archived_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func ToChannel(v usecase.ChannelView) Channel {
	dm := v.DMUserIDs
	if dm == nil {
		dm = []string{}
	}
	return Channel{
		ID: v.ID, Type: string(v.Type), Name: v.Name, Topic: v.Topic, LastSeq: v.LastSeq, LastReadSeq: v.LastReadSeq,
		UnreadCount: v.UnreadCount, MentionCount: v.MentionCount, MemberCount: v.MemberCount, DMUserIDs: dm,
		ArchivedAt: v.ArchivedAt, CreatedAt: v.CreatedAt.UTC(),
	}
}

// Attachment is a file on a message. Uploads arrive with M2-05.
type Attachment struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
	IsImage  bool   `json:"is_image"`
}

// Reaction counts one emoji on a message. Reactions arrive with M2-05.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int64  `json:"count"`
	Me    bool   `json:"me"`
}

// Message is a message of a channel.
type Message struct {
	ID              string          `json:"id"`
	ChannelID       string          `json:"channel_id"`
	Seq             int64           `json:"seq"`
	SenderID        string          `json:"sender_id"`
	ClientMsgID     string          `json:"client_msg_id"`
	Kind            string          `json:"kind" doc:"text, or a card kind such as approval.request"`
	Body            string          `json:"body" doc:"Empty once deleted"`
	Payload         json.RawMessage `json:"payload" nullable:"true" doc:"Card payload; null for text"`
	FallbackText    string          `json:"fallback_text"`
	ThreadRootID    *string         `json:"thread_root_id"`
	ReplyCount      int64           `json:"reply_count"`
	Attachments     []Attachment    `json:"attachments" nullable:"false"`
	Reactions       []Reaction      `json:"reactions" nullable:"false"`
	Mentions        []string        `json:"mentions" nullable:"false" doc:"User IDs mentioned who are in the channel"`
	MentionsChannel bool            `json:"mentions_channel"`
	EditedAt        *time.Time      `json:"edited_at"`
	DeletedAt       *time.Time      `json:"deleted_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

func ToMessage(v usecase.MessageView) Message {
	m := v.Message
	out := Message{
		ID: m.ID, ChannelID: m.ChannelID, Seq: m.Seq, SenderID: m.SenderID, ClientMsgID: m.ClientMsgID, Kind: m.Kind,
		Body: m.Body, Payload: m.Payload, FallbackText: m.FallbackText, ReplyCount: v.ReplyCount,
		Attachments: []Attachment{}, Reactions: []Reaction{}, Mentions: m.Mentions, MentionsChannel: m.MentionsChannel,
		EditedAt: m.EditedAt, DeletedAt: m.DeletedAt, CreatedAt: m.CreatedAt.UTC(),
	}
	if out.Mentions == nil {
		out.Mentions = []string{}
	}
	if m.ThreadRootID != "" {
		root := m.ThreadRootID
		out.ThreadRootID = &root
	}
	return out
}

// Member is a channel member.
type Member struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name" doc:"Empty when the auth module is unavailable"`
	JoinedAt    time.Time `json:"joined_at"`
}

func ToMember(m usecase.Member) Member {
	return Member{UserID: m.UserID, DisplayName: m.DisplayName, JoinedAt: m.JoinedAt.UTC()}
}

