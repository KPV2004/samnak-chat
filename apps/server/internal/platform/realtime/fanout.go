package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Redis names (plan, WebSocket protocol).
const (
	channelUsers   = "rt:users"
	channelControl = "rt:control"
	presencePrefix = "rt:online:"
	presenceTTL    = 60 * time.Second

	subscribeTimeout = 5 * time.Second
	redisOpTimeout   = 2 * time.Second
	pubsubBuffer     = 1024
)

func presenceKey(userID string) string { return presencePrefix + userID }

// usersMsg is published on rt:users. E is the ready-to-send frame, so
// receiving instances never re-encode it.
type usersMsg struct {
	Origin string          `json:"o"`
	Users  []string        `json:"u"`
	Frame  json.RawMessage `json:"e"`
}

// controlMsg is published on rt:control.
type controlMsg struct {
	Origin   string `json:"o"`
	DeviceID string `json:"d"`
}

// fanout connects a hub to Redis pub/sub. Each instance delivers to its own
// connections; the publisher already delivered locally, so it ignores its own
// messages.
type fanout struct {
	hub    *Hub
	rdb    *goredis.Client
	ps     *goredis.PubSub
	cancel context.CancelFunc
	done   chan struct{}
}

func newFanout(h *Hub) *fanout {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fanout{hub: h, rdb: h.rdb, cancel: cancel, done: make(chan struct{})}
	f.ps = h.rdb.Subscribe(ctx, channelUsers, channelControl)

	// Wait for both subscription confirmations, so a message published right
	// after NewHub returns is not missed. go-redis reconnects by itself, so a
	// failure here is only logged.
	wait, stop := context.WithTimeout(ctx, subscribeTimeout)
	for range 2 {
		if _, err := f.ps.Receive(wait); err != nil {
			h.log.Warn("realtime: redis subscribe", "error", err)
			break
		}
	}
	stop()

	ch := f.ps.Channel(goredis.WithChannelSize(pubsubBuffer))
	go f.loop(ch)
	return f
}

func (f *fanout) loop(ch <-chan *goredis.Message) {
	defer close(f.done)
	for msg := range ch {
		switch msg.Channel {
		case channelUsers:
			var m usersMsg
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				f.hub.log.Warn("realtime: bad rt:users message", "error", err)
				continue
			}
			if m.Origin != f.hub.id {
				f.hub.deliver(m.Users, m.Frame)
			}
		case channelControl:
			var m controlMsg
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				f.hub.log.Warn("realtime: bad rt:control message", "error", err)
				continue
			}
			if m.Origin != f.hub.id {
				f.hub.disconnectDevice(m.DeviceID)
			}
		}
	}
}

func (f *fanout) publishUsers(ctx context.Context, userIDs []string, frame []byte) error {
	payload, err := json.Marshal(usersMsg{Origin: f.hub.id, Users: userIDs, Frame: frame})
	if err != nil {
		return fmt.Errorf("realtime: marshal rt:users: %w", err)
	}
	if err := f.rdb.Publish(ctx, channelUsers, payload).Err(); err != nil {
		return fmt.Errorf("realtime: publish rt:users: %w", err)
	}
	return nil
}

func (f *fanout) publishDisconnect(ctx context.Context, deviceID string) error {
	payload, err := json.Marshal(controlMsg{Origin: f.hub.id, DeviceID: deviceID})
	if err != nil {
		return fmt.Errorf("realtime: marshal rt:control: %w", err)
	}
	if err := f.rdb.Publish(ctx, channelControl, payload).Err(); err != nil {
		return fmt.Errorf("realtime: publish rt:control: %w", err)
	}
	return nil
}

func (f *fanout) close() {
	f.cancel()
	_ = f.ps.Close()
	<-f.done
}

// touchPresence marks the user online for presenceTTL. Presence is not
// removed when the last connection closes: the key expires, so a user
// connected to another instance never appears offline by mistake.
func (h *Hub) touchPresence(ctx context.Context, userID string) {
	if h.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	if err := h.rdb.Set(ctx, presenceKey(userID), 1, presenceTTL).Err(); err != nil {
		h.log.Warn("realtime: refresh presence", "user_id", userID, "error", err)
	}
}

// Online returns the users, in the order given, with a connection on any
// instance.
func (h *Hub) Online(ctx context.Context, userIDs []string) ([]string, error) {
	if len(userIDs) == 0 {
		return []string{}, nil
	}
	online := make([]string, 0, len(userIDs))
	if h.rdb == nil {
		h.mu.RLock()
		defer h.mu.RUnlock()
		for _, uid := range userIDs {
			if len(h.byUser[uid]) > 0 {
				online = append(online, uid)
			}
		}
		return online, nil
	}
	keys := make([]string, len(userIDs))
	for i, uid := range userIDs {
		keys[i] = presenceKey(uid)
	}
	vals, err := h.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("realtime: read presence: %w", err)
	}
	for i, v := range vals {
		if v != nil {
			online = append(online, userIDs[i])
		}
	}
	return online, nil
}
