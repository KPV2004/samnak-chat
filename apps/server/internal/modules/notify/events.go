package notify

import "github.com/KPV2004/samnak-chat/apps/server/internal/modules/notify/adapter/wire"

// WebSocket frames notify sends to a user's own devices (NOTI-03). They are
// not bus events: the bus carries what modules did, these tell one user about
// their inbox. Clients find the same rows over REST after reconnecting.
const (
	// EventCreated is `notification.created`, one frame per stored row.
	EventCreated = wire.EventCreated
	// EventRead is `notification.read`, sent when notifications were marked
	// read on another device.
	EventRead = wire.EventRead
)

// CreatedEvent is the `data` of notification.created.
type CreatedEvent = wire.CreatedEvent

// ReadEvent is the `data` of notification.read.
type ReadEvent = wire.ReadEvent
