// Package task owns tasks: work items that live in a channel, have an
// assignee, a due date and a status, and are shown as a card in the chat
// (SRS TASK-01..08). This file and events.go are the only parts other modules
// may import.
//
// Other modules never call task. They listen for the events in events.go and
// the card's thread is the task's comment thread (TASK-07).
package task

import "github.com/KPV2004/samnak-chat/apps/server/internal/modules/task/usecase"

// CardKind is the kind of the task card that chat stores (`task.item`).
const CardKind = usecase.CardKind

// CardPayload is the payload of a task.item card, for clients and tests that
// decode it.
type CardPayload = usecase.CardPayload

// Notification kinds the module produces.
const (
	NotifyAssigned = usecase.KindAssigned
	NotifyDueSoon  = usecase.KindDueSoon
	NotifyOverdue  = usecase.KindOverdue
)

// The chat extensions the module registers (CHAT-16).
const (
	// CommandName is the slash command `/task <title> [<@user>]`.
	CommandName = "task"
	// ActionCreateFromMessage is the message menu item "Create task". The
	// client opens its form with the message text as the title and the
	// message ID as source_message_id.
	ActionCreateFromMessage = "task.create_from_message"
)
