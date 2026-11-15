package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

// conn is one WebSocket connection: the handler goroutine is the reader and a
// second goroutine is the writer, fed by the bounded send queue.
type conn struct {
	hub  *Hub
	ws   *websocket.Conn
	id   app.Identity // set after a successful handshake
	send chan []byte
	done chan struct{} // closed when the reader returns; stops the writer

	ctx    context.Context
	cancel context.CancelFunc

	timer    *time.Timer // auth deadline, then idle deadline
	kickOnce sync.Once
	kicked   atomic.Bool
	kickDone chan struct{} // closed once the close handshake has finished

	lastPresence time.Time // reader goroutine only
}

func newConn(h *Hub, ws *websocket.Conn) *conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &conn{
		hub:      h,
		ws:       ws,
		send:     make(chan []byte, h.sendQueue),
		done:     make(chan struct{}),
		kickDone: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// kick closes the connection with a close frame. It never blocks, can be
// called from any goroutine and acts once. The reader notices through the
// failed read.
func (c *conn) kick(code websocket.StatusCode, reason string) {
	c.kickOnce.Do(func() {
		c.kicked.Store(true)
		go func() {
			defer close(c.kickDone)
			_ = c.ws.Close(code, reason)
		}()
	})
}

// drop closes the socket without a close handshake. A slow client cannot be
// expected to answer one, and waiting for it would hold the writer.
func (c *conn) drop() {
	c.kickOnce.Do(func() {
		c.kicked.Store(true)
		go func() {
			defer close(c.kickDone)
			_ = c.ws.CloseNow()
		}()
	})
}

// enqueue queues a frame without blocking. A client whose queue is full is
// too slow and gets disconnected, so it can never hold up other users.
func (c *conn) enqueue(frame []byte) {
	select {
	case c.send <- frame:
	default:
		c.hub.log.Warn("realtime: slow client disconnected", "user_id", c.id.UserID, "device_id", c.id.DeviceID)
		c.drop()
	}
}

func (c *conn) run() {
	h := c.hub
	c.timer = time.AfterFunc(h.authTimeout, func() { c.kick(codeUnauthorized, "authentication timeout") })
	writerDone := make(chan struct{})
	defer func() {
		c.timer.Stop()
		close(c.done)
		c.cancel()
		h.unregister(c)
		<-writerDone
		if c.kicked.Load() {
			<-c.kickDone // let the close frame go out before dropping the socket
		}
		_ = c.ws.CloseNow()
	}()

	if !c.handshake() {
		close(writerDone)
		return
	}
	go func() {
		defer close(writerDone)
		c.writeLoop()
	}()
	c.readLoop()
}

// handshake reads the first frame and authenticates it. On success the
// connection is registered and "ready" is the first frame in its queue.
func (c *conn) handshake() bool {
	h := c.hub
	typ, data, err := c.ws.Read(c.ctx)
	if err != nil {
		return false
	}
	if typ != websocket.MessageText {
		c.kick(websocket.StatusUnsupportedData, "text frames only")
		return false
	}
	var env app.Envelope
	var ad authData
	if json.Unmarshal(data, &env) != nil || env.Type != typeAuth ||
		json.Unmarshal(env.Data, &ad) != nil || ad.AccessToken == "" {
		c.kick(codeUnauthorized, "authentication required")
		return false
	}
	auth, ok := h.authenticator()
	if !ok {
		c.kick(codeUnauthorized, "authentication unavailable")
		return false
	}
	ctx, cancel := context.WithTimeout(c.ctx, authCallTimeout)
	id, err := auth.Authenticate(ctx, ad.AccessToken)
	cancel()
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		c.kick(codeUnauthorized, "unauthorized")
		return false
	case err != nil:
		h.log.Error("realtime: authenticate", "error", err)
		c.kick(websocket.StatusInternalError, "authentication failed")
		return false
	}
	c.id = id
	c.send <- marshalEnvelope(typeReady, readyData{UserID: id.UserID, DeviceID: id.DeviceID})
	h.register(c)
	c.timer.Stop()
	c.timer = time.AfterFunc(h.idleTimeout, func() { c.kick(codeIdle, "idle timeout") })
	h.touchPresence(c.ctx, id.UserID)
	h.log.Debug("realtime: connected", "user_id", id.UserID, "device_id", id.DeviceID)
	return true
}

func (c *conn) readLoop() {
	for {
		typ, data, err := c.ws.Read(c.ctx)
		if err != nil {
			return
		}
		c.timer.Reset(c.hub.idleTimeout)
		if typ != websocket.MessageText {
			c.kick(websocket.StatusUnsupportedData, "text frames only")
			return
		}
		c.dispatch(data)
	}
}

func (c *conn) writeLoop() {
	for {
		select {
		case frame := <-c.send:
			ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			err := c.ws.Write(ctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				c.drop()
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *conn) dispatch(data []byte) {
	var env app.Envelope
	if err := json.Unmarshal(data, &env); err != nil || env.Type == "" {
		c.replyError("", "invalid", "malformed message")
		return
	}
	switch env.Type {
	case typePing:
		c.enqueue(pongFrame)
		if now := time.Now(); now.Sub(c.lastPresence) >= minPresenceGap {
			c.lastPresence = now
			c.hub.touchPresence(c.ctx, c.id.UserID)
		}
	case typeAuth:
		c.replyError(env.Type, "invalid", "already authenticated")
	default:
		fn, ok := c.hub.handler(env.Type)
		if !ok {
			c.replyError(env.Type, "unknown_type", "unknown message type")
			return
		}
		if err := c.call(fn, env); err != nil {
			code, msg := errorCode(err)
			if code == "internal" {
				c.hub.log.Error("realtime: handler failed", "type", env.Type, "user_id", c.id.UserID, "error", err)
			}
			c.replyError(env.Type, code, msg)
		}
	}
}

// call runs a handler and turns a panic into an error, so one bad handler
// does not take the connection down.
func (c *conn) call(fn app.ClientHandler, env app.Envelope) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return fn(c.ctx, c.id, env.Data)
}

func (c *conn) replyError(typ, code, msg string) {
	c.enqueue(marshalEnvelope(typeError, errorData{Type: typ, Code: code, Message: msg}))
}
