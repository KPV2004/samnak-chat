// Package chat owns channels, messages, threads and read state (SRS section 7).
// This file and events.go are the only parts other modules may import.
//
// Other modules post cards through app.CardRegistry (look it up with
// app.Lookup in Init) and listen for MessageCreated.
package chat

import "github.com/KPV2004/samnak-chat/apps/server/internal/modules/chat/domain"

// Actions of the chat module for workspace.Authorizer. The module registers
// them in Init; their role sets are fixed there.
const (
	ActionChannelCreate   = domain.ActionChannelCreate
	ActionChannelManage   = domain.ActionChannelManage
	ActionMessageModerate = domain.ActionMessageModerate
)
