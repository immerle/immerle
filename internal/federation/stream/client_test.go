package stream_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/federation/stream"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// fakeHub is a minimal stand-in for the hub's stream endpoint: it accepts one
// connection, records every frame it receives, and lets the test push frames
// down to the client.
type fakeHub struct {
	t    *testing.T
	conn *websocket.Conn

	mu       sync.Mutex
	received []stream.Frame
}

func newFakeHub(t *testing.T) (*httptest.Server, *fakeHub) {
	t.Helper()
	fh := &fakeHub{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		fh.mu.Lock()
		fh.conn = c
		fh.mu.Unlock()
		for {
			_, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			var f stream.Frame
			if err := json.Unmarshal(data, &f); err != nil {
				continue
			}
			fh.mu.Lock()
			fh.received = append(fh.received, f)
			fh.mu.Unlock()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, fh
}

func (fh *fakeHub) send(t *testing.T, f stream.Frame) {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var conn *websocket.Conn
	for conn == nil {
		if time.Now().After(deadline) {
			t.Fatal("client never connected")
		}
		fh.mu.Lock()
		conn = fh.conn
		fh.mu.Unlock()
		if conn == nil {
			time.Sleep(time.Millisecond)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func (fh *fakeHub) framesOfType(typ string) []stream.Frame {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	var out []stream.Frame
	for _, f := range fh.received {
		if f.Type == typ {
			out = append(out, f)
		}
	}
	return out
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never met")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestClientDispatch(t *testing.T) {
	srv, fh := newFakeHub(t)
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1)

	upserts := make(chan stream.Frame, 4)
	deletes := make(chan stream.Frame, 4)
	replays := make(chan stream.Frame, 4)

	c := stream.New(
		func() hub.Auth { return hub.Auth{InstanceID: "inst-1", PrivateKey: "key-1"} },
		func() string { return wsURL },
		func(context.Context) (map[string]string, error) { return map[string]string{"pub-1": "v1"}, nil },
		stream.Handlers{
			OnUpsert: func(_ context.Context, f stream.Frame) error { upserts <- f; return nil },
			OnDelete: func(_ context.Context, f stream.Frame) error { deletes <- f; return nil },
			OnReplay: func(_ context.Context, f stream.Frame) error { replays <- f; return nil },
		},
		testLogger(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	// welcome -> the client answers with a resume built from our cursors func.
	fh.send(t, stream.Frame{Type: stream.TypeWelcome})
	waitFor(t, func() bool { return len(fh.framesOfType(stream.TypeResume)) > 0 })
	resume := fh.framesOfType(stream.TypeResume)[0]
	if resume.Cursors["pub-1"] != "v1" {
		t.Fatalf("resume cursors = %v, want pub-1:v1", resume.Cursors)
	}

	fh.send(t, stream.Frame{Type: stream.TypePlaylistUpsert, AuthorID: "pub-1", ExternalID: "ext-1", Version: "v2"})
	select {
	case f := <-upserts:
		if f.ExternalID != "ext-1" || f.AuthorID != "pub-1" {
			t.Fatalf("got upsert %+v, want ext-1/pub-1", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnUpsert not called")
	}

	fh.send(t, stream.Frame{Type: stream.TypePlaylistDelete, AuthorID: "pub-1", ExternalID: "ext-1"})
	select {
	case f := <-deletes:
		if f.ExternalID != "ext-1" {
			t.Fatalf("got delete %+v, want ext-1", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnDelete not called")
	}

	fh.send(t, stream.Frame{Type: stream.TypeReplayRequest, ForSubscriberID: "sub-1", SinceVersion: "v0"})
	select {
	case f := <-replays:
		if f.ForSubscriberID != "sub-1" {
			t.Fatalf("got replay %+v, want sub-1", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnReplay not called")
	}
}
