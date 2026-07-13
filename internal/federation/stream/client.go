package stream

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/immerle/immerle/internal/federation/hub"
)

// ErrNotConnected is returned by Send when the socket is currently down; the
// caller (playlist push, RFC §7) maps it to outbox.ErrNotReady so the job is
// retried once reconnected instead of falling back to REST.
var ErrNotConnected = errors.New("federation stream: not connected")

// maxFramePayload mirrors the hub's own limit (internal/ws.MaxFramePayload) so
// a locally-built oversized frame fails fast instead of getting the connection
// dropped by the hub.
const maxFramePayload = 1 << 20 // 1 MiB

const (
	heartbeatInterval = 25 * time.Second
	writeTimeout      = 10 * time.Second
	baseBackoff       = time.Second
	maxBackoff        = 5 * time.Minute
	// healthyConnection is how long a connection must have stayed up before a
	// subsequent disconnect resets the backoff counter, instead of compounding
	// it as if every reconnect attempt had failed outright.
	healthyConnection = 30 * time.Second
)

// Handlers dispatches inbound frames to the federation service. Each may be
// nil to ignore that frame type (e.g. OnReplay is unset until the socket push
// path — RFC §6/§7 — lands).
type Handlers struct {
	OnUpsert func(ctx context.Context, f Frame) error
	OnDelete func(ctx context.Context, f Frame) error
	OnReplay func(ctx context.Context, f Frame) error
}

// Client is the instance-side socket to the hub's federation feed
// (GET /api/v1/instances/me/stream): it dials, reconnects with backoff, sends
// resume/heartbeat, and dispatches inbound frames to Handlers.
type Client struct {
	auth    func() hub.Auth
	hubURL  func() string
	cursors func(ctx context.Context) (map[string]string, error)
	h       Handlers
	logger  *slog.Logger

	mu   sync.Mutex
	conn *websocket.Conn // current active connection; nil while disconnected
}

// New builds a Client. auth and hubURL are read live (same pattern as
// federation.Service), so relinking or changing the hub URL takes effect on
// the next reconnect without a restart. cursors builds the resume payload —
// the last applied version per followed source instance.
func New(auth func() hub.Auth, hubURL func() string, cursors func(context.Context) (map[string]string, error), h Handlers, logger *slog.Logger) *Client {
	return &Client{auth: auth, hubURL: hubURL, cursors: cursors, h: h, logger: logger}
}

// Run dials and serves the socket until ctx is cancelled, reconnecting with an
// exponential backoff (jittered) on every disconnect. It never returns before
// ctx is done.
func (c *Client) Run(ctx context.Context) {
	attempt := 0
	for {
		connectedAt := time.Now()
		if err := c.connectAndServe(ctx); err != nil && ctx.Err() == nil {
			c.logger.Warn("federation stream disconnected", "error", err)
		}
		if ctx.Err() != nil {
			return
		}
		if time.Since(connectedAt) >= healthyConnection {
			attempt = 0
		} else {
			attempt++
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoffWithJitter(attempt)):
		}
	}
}

// backoffWithJitter is an exponential delay (1s, 2s, 4s … capped at
// maxBackoff) with up to 50% jitter, so many instances reconnecting after a
// hub restart don't all retry in lockstep (thundering herd).
func backoffWithJitter(attempt int) time.Duration {
	shift := attempt
	if shift > 8 {
		shift = 8
	}
	d := baseBackoff << uint(shift)
	if d <= 0 || d > maxBackoff {
		d = maxBackoff
	}
	return d/2 + time.Duration(rand.Int63n(int64(d)/2+1))
}

// connectAndServe dials the socket and blocks dispatching frames until the
// connection drops or ctx is cancelled.
func (c *Client) connectAndServe(ctx context.Context) error {
	a := c.auth()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+a.PrivateKey)
	header.Set("X-Instance-ID", a.InstanceID)

	conn, resp, err := websocket.Dial(ctx, c.hubURL()+"/api/v1/instances/me/stream", &websocket.DialOptions{HTTPHeader: header})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxFramePayload)

	c.setConn(conn)
	defer c.setConn(nil)

	hbCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go c.heartbeat(hbCtx, conn)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var f Frame
		if err := json.Unmarshal(data, &f); err != nil {
			continue // malformed frame from an incompatible/misbehaving hub; ignore
		}
		if err := c.dispatch(ctx, conn, f); err != nil {
			c.logger.Warn("federation stream: handling frame failed", "type", f.Type, "error", err)
		}
	}
}

func (c *Client) dispatch(ctx context.Context, conn *websocket.Conn, f Frame) error {
	switch f.Type {
	case TypeWelcome:
		return c.sendResume(ctx, conn)
	case TypePlaylistUpsert:
		if c.h.OnUpsert != nil {
			return c.h.OnUpsert(ctx, f)
		}
	case TypePlaylistDelete:
		if c.h.OnDelete != nil {
			return c.h.OnDelete(ctx, f)
		}
	case TypeReplayRequest:
		if c.h.OnReplay != nil {
			return c.h.OnReplay(ctx, f)
		}
	case TypeHeartbeatAck:
		// nothing to do; receiving it just confirms liveness.
	case TypeError:
		c.logger.Warn("federation stream: hub reported error", "code", f.Code, "message", f.Message)
	}
	return nil
}

// sendResume declares, for every followed source instance, the last version
// already applied locally — the hub translates each entry into a
// replay.request forwarded to that publisher, if connected (RFC hub §8).
func (c *Client) sendResume(ctx context.Context, conn *websocket.Conn) error {
	cursors, err := c.cursors(ctx)
	if err != nil {
		return err
	}
	return c.send(ctx, conn, Frame{Type: TypeResume, Cursors: cursors})
}

func (c *Client) heartbeat(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.send(ctx, conn, Frame{Type: TypeHeartbeat}); err != nil {
				return
			}
		}
	}
}

func (c *Client) send(ctx context.Context, conn *websocket.Conn, f Frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, data)
}

func (c *Client) setConn(conn *websocket.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

// Send pushes a frame (e.g. playlist.upsert/delete, or a replay reply with
// Target set) over the currently active connection. Writes are safe to call
// concurrently with the read loop and with each other (the underlying library
// guarantees this — see websocket.Conn's doc comment). Returns
// ErrNotConnected while the socket is down.
func (c *Client) Send(ctx context.Context, f Frame) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}
	return c.send(ctx, conn, f)
}
