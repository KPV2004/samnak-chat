package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// Hub is the WebSocket hub. It implements app.Realtime and knows no feature.
//
// It keeps each user's connections in this instance's memory. With a Redis
// client, SendToUsers and DisconnectDevice also reach other instances, and
// presence is shared. Without one (nil client) the hub serves a single
// instance.
type Hub struct {
	log      *slog.Logger
	services *app.Services
	rdb      *goredis.Client
	id       string // identifies this instance on the Redis channels

	authTimeout    time.Duration
	idleTimeout    time.Duration
	sendQueue      int
	originPatterns []string

	hmu      sync.RWMutex
	handlers map[string]app.ClientHandler

	mu       sync.RWMutex
	closed   bool
	conns    map[*conn]struct{}            // every open connection, authenticated or not
	byUser   map[string]map[*conn]struct{} // authenticated connections
	byDevice map[string]map[*conn]struct{}
	wg       sync.WaitGroup

	bus *fanout // nil without Redis
}

var _ app.Realtime = (*Hub)(nil)

// Option tunes a Hub. The defaults are the values in the M2 plan.
type Option func(*Hub)

// WithAuthTimeout sets how long a client has to send "auth" (default 10 s).
func WithAuthTimeout(d time.Duration) Option { return func(h *Hub) { h.authTimeout = d } }

// WithIdleTimeout sets how long a connection may stay silent (default 60 s).
func WithIdleTimeout(d time.Duration) Option { return func(h *Hub) { h.idleTimeout = d } }

// WithSendQueue sets the per-connection outbound queue size (default 256).
func WithSendQueue(n int) Option { return func(h *Hub) { h.sendQueue = n } }

// WithOriginPatterns turns on the browser Origin check for the handshake.
// Without it any origin may connect, which is safe here because the socket
// carries no ambient credentials: the client must send an access token.
func WithOriginPatterns(patterns ...string) Option {
	return func(h *Hub) { h.originPatterns = patterns }
}

// NewHub builds a hub. The app.Authenticator is looked up in services at
// handshake time, because the auth module provides it after the hub exists.
// rdb may be nil for a single instance.
func NewHub(logger *slog.Logger, services *app.Services, rdb *goredis.Client, opts ...Option) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{
		log:         logger,
		services:    services,
		rdb:         rdb,
		id:          uuid.NewString(),
		authTimeout: defaultAuthTimeout,
		idleTimeout: defaultIdleTimeout,
		sendQueue:   defaultSendQueue,
		handlers:    map[string]app.ClientHandler{},
		conns:       map[*conn]struct{}{},
		byUser:      map[string]map[*conn]struct{}{},
		byDevice:    map[string]map[*conn]struct{}{},
	}
	for _, o := range opts {
		o(h)
	}
	if rdb != nil {
		h.bus = newFanout(h)
	}
	return h
}

// Mount registers GET /api/v1/ws.
func (h *Hub) Mount(r chi.Router) {
	r.Get("/api/v1/ws", h.serveWS)
}

// Handle registers h for a client message type. It panics on a reserved or
// already registered type, which is a wiring mistake found at startup.
func (h *Hub) Handle(msgType string, fn app.ClientHandler) {
	if msgType == typeAuth || msgType == typePing {
		panic(fmt.Sprintf("realtime: message type %q is reserved by the hub", msgType))
	}
	h.hmu.Lock()
	defer h.hmu.Unlock()
	if _, dup := h.handlers[msgType]; dup {
		panic(fmt.Sprintf("realtime: message type %q is already handled", msgType))
	}
	h.handlers[msgType] = fn
}

func (h *Hub) handler(msgType string) (app.ClientHandler, bool) {
	h.hmu.RLock()
	defer h.hmu.RUnlock()
	fn, ok := h.handlers[msgType]
	return fn, ok
}

// Connections returns the number of open connections on this instance.
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// SendToUsers delivers env to every open connection of each user, on this
// instance and, with Redis, on every other one.
func (h *Hub) SendToUsers(ctx context.Context, userIDs []string, env app.Envelope) error {
	if env.Type == "" {
		return fmt.Errorf("realtime: envelope type is required: %w", app.ErrInvalid)
	}
	if len(userIDs) == 0 {
		return nil
	}
	frame, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("realtime: marshal envelope: %w", err)
	}
	h.deliver(userIDs, frame)
	if h.bus != nil {
		return h.bus.publishUsers(ctx, userIDs, frame)
	}
	return nil
}

// DisconnectDevice closes every connection of the device on every instance.
func (h *Hub) DisconnectDevice(ctx context.Context, deviceID string) error {
	h.disconnectDevice(deviceID)
	if h.bus != nil {
		return h.bus.publishDisconnect(ctx, deviceID)
	}
	return nil
}

// DisconnectUser closes every connection of the user on every instance. It is
// what a revoke of all devices needs (AUTH-08); app.Realtime only knows
// devices, so the composition root asserts this method on the hub.
func (h *Hub) DisconnectUser(ctx context.Context, userID string) error {
	h.disconnectUser(userID)
	if h.bus != nil {
		return h.bus.publishDisconnectUser(ctx, userID)
	}
	return nil
}

// deliver queues frame on each local connection of the users. The frame is
// shared between connections and must not be modified.
func (h *Hub) deliver(userIDs []string, frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, uid := range userIDs {
		for c := range h.byUser[uid] {
			c.enqueue(frame)
		}
	}
}

func (h *Hub) disconnectDevice(deviceID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.byDevice[deviceID] {
		c.kick(codeUnauthorized, "device logged out")
	}
}

// disconnectUser closes every local connection of the user, whichever device
// it came from.
func (h *Hub) disconnectUser(userID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.byUser[userID] {
		c.kick(codeUnauthorized, "logged out")
	}
}

// Close stops accepting connections, closes the open ones with "going away"
// and waits for them, or for ctx. It does not close the Redis client.
func (h *Hub) Close(ctx context.Context) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	open := make([]*conn, 0, len(h.conns))
	for c := range h.conns {
		open = append(open, c)
	}
	h.mu.Unlock()

	for _, c := range open {
		c.kick(websocket.StatusGoingAway, "server shutting down")
	}
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()
	var err error
	select {
	case <-done:
	case <-ctx.Done():
		err = fmt.Errorf("realtime: close: %w", ctx.Err())
	}
	if h.bus != nil {
		h.bus.close()
	}
	return err
}

func (h *Hub) serveWS(w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{OriginPatterns: h.originPatterns}
	if len(h.originPatterns) == 0 {
		opts.InsecureSkipVerify = true
	}
	ws, err := websocket.Accept(w, r, opts)
	if err != nil {
		return // Accept has already written the HTTP error
	}
	ws.SetReadLimit(maxInboundBytes)

	c := newConn(h, ws)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		_ = ws.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	h.conns[c] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()
	defer h.wg.Done()

	c.run()
}

// register makes an authenticated connection reachable by user and device.
func (h *Hub) register(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	add(h.byUser, c.id.UserID, c)
	add(h.byDevice, c.id.DeviceID, c)
}

func (h *Hub) unregister(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, c)
	remove(h.byUser, c.id.UserID, c)
	remove(h.byDevice, c.id.DeviceID, c)
}

func add(m map[string]map[*conn]struct{}, key string, c *conn) {
	set := m[key]
	if set == nil {
		set = map[*conn]struct{}{}
		m[key] = set
	}
	set[c] = struct{}{}
}

func remove(m map[string]map[*conn]struct{}, key string, c *conn) {
	set := m[key]
	delete(set, c)
	if len(set) == 0 {
		delete(m, key)
	}
}

func (h *Hub) authenticator() (app.Authenticator, bool) {
	if h.services == nil {
		return nil, false
	}
	return app.Lookup[app.Authenticator](h.services)
}
