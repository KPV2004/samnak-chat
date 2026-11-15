package chat

// MessageCreated is published after a message was committed to a channel
// (CHAT-07). Listeners such as notify use it for DMs and mentions. Replaying
// the same client_msg_id does not publish it again.
type MessageCreated struct {
	WorkspaceID     string   `json:"workspace_id"`
	ChannelID       string   `json:"channel_id"`
	MessageID       string   `json:"message_id"`
	Seq             int64    `json:"seq"`
	SenderID        string   `json:"sender_id"`
	ChannelType     string   `json:"channel_type"` // public, private or dm
	ThreadRootID    string   `json:"thread_root_id"`
	Mentions        []string `json:"mentions"`
	MentionsChannel bool     `json:"mentions_channel"`
}

func (MessageCreated) EventName() string { return "chat.message_created" }
