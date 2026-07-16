package immerle

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestJamMine covers the header button's "do I already have a Jam running"
// check: 404 with no session hosted, the session once one exists.
func TestJamMine(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")

	if st := doStatus(t, srv, http.MethodGet, "/jam/mine", alice, nil); st != http.StatusNotFound {
		t.Fatalf("no session hosted: expected 404, got %d", st)
	}

	st, body := doMap(t, srv, http.MethodPost, "/jam", alice, map[string]any{"name": "party"})
	if st != http.StatusCreated {
		t.Fatalf("create jam: status %d", st)
	}
	session, _ := body["session"].(map[string]any)
	id, _ := session["id"].(string)

	st, mine := doMap(t, srv, http.MethodGet, "/jam/mine", alice, nil)
	if st != http.StatusOK {
		t.Fatalf("jam/mine: status %d", st)
	}
	mineSession, _ := mine["session"].(map[string]any)
	if mineSession["id"] != id {
		t.Fatalf("expected hosted session %q, got %+v", id, mineSession)
	}
}

// TestJamInviteLifecycle covers inviting a user (host-only), the invitee
// seeing and dismissing it, and a join auto-clearing the invite.
func TestJamInviteLifecycle(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")

	st, body := doMap(t, srv, http.MethodPost, "/jam", alice, map[string]any{"name": "party"})
	if st != http.StatusCreated {
		t.Fatalf("create jam: status %d", st)
	}
	session, _ := body["session"].(map[string]any)
	id, _ := session["id"].(string)

	// A non-host cannot invite.
	if st := doStatus(t, srv, http.MethodPost, "/jam/"+id+"/invites", bob, map[string]any{"username": "bob"}); st != http.StatusForbidden {
		t.Fatalf("non-host invite: expected 403, got %d", st)
	}

	// The host invites bob.
	if st := doStatus(t, srv, http.MethodPost, "/jam/"+id+"/invites", alice, map[string]any{"username": "bob"}); st != http.StatusNoContent {
		t.Fatalf("invite: expected 204, got %d", st)
	}

	// Bob sees the pending invite.
	st, invitesBody := doMap(t, srv, http.MethodGet, "/jam/invites/mine", bob, nil)
	if st != http.StatusOK {
		t.Fatalf("invites/mine: status %d", st)
	}
	invites, _ := invitesBody["invites"].([]any)
	if len(invites) != 1 {
		t.Fatalf("expected 1 pending invite, got %d: %+v", len(invites), invites)
	}
	inv := invites[0].(map[string]any)
	if inv["sessionId"] != id || inv["inviterUsername"] != "alice" {
		t.Fatalf("unexpected invite: %+v", inv)
	}
	inviteID, _ := inv["id"].(string)

	// Bob dismisses it.
	if st := doStatus(t, srv, http.MethodDelete, "/jam/invites/"+inviteID, bob, nil); st != http.StatusNoContent {
		t.Fatalf("dismiss: expected 204, got %d", st)
	}
	st, invitesBody = doMap(t, srv, http.MethodGet, "/jam/invites/mine", bob, nil)
	if st != http.StatusOK {
		t.Fatalf("invites/mine after dismiss: status %d", st)
	}
	if invites, _ := invitesBody["invites"].([]any); len(invites) != 0 {
		t.Fatalf("expected no pending invites after dismiss, got %+v", invites)
	}

	// Re-inviting, then joining directly, also clears the invite (accepting is
	// just joining — no separate "accept" endpoint).
	if st := doStatus(t, srv, http.MethodPost, "/jam/"+id+"/invites", alice, map[string]any{"username": "bob"}); st != http.StatusNoContent {
		t.Fatalf("re-invite: expected 204, got %d", st)
	}
	if st := doStatus(t, srv, http.MethodPost, "/jam/"+id+"/participants", bob, nil); st != http.StatusCreated {
		t.Fatalf("join: expected 201, got %d", st)
	}
	st, invitesBody = doMap(t, srv, http.MethodGet, "/jam/invites/mine", bob, nil)
	if st != http.StatusOK {
		t.Fatalf("invites/mine after join: status %d", st)
	}
	if invites, _ := invitesBody["invites"].([]any); len(invites) != 0 {
		t.Fatalf("expected joining to clear the pending invite, got %+v", invites)
	}
}

// TestJamInvitesPushOverPlayQueueSSE covers real-time delivery: Jam invites
// ride the same always-open /play-queue/events connection as play-queue sync
// (a dedicated /jam/invites/events stream was cut — see handleStreamPlayQueue's
// doc comment — because every extra always-on SSE connection eats into the
// browser's ~6-per-origin cap under HTTP/1.1, and made the app noticeably
// laggy while hosting a Jam). The invitee's stream gets an "invites" event
// with an empty snapshot on connect, then a fresh one the moment the host
// invites them — alongside the unrelated "state" (play-queue) events on the
// same connection.
func TestJamInvitesPushOverPlayQueueSSE(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")

	st, body := doMap(t, srv, http.MethodPost, "/jam", alice, map[string]any{"name": "party"})
	if st != http.StatusCreated {
		t.Fatalf("create jam: status %d", st)
	}
	session, _ := body["session"].(map[string]any)
	id, _ := session["id"].(string)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/play-queue/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+bob)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events: status %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	// readInvitesEvent skips over unrelated "state" (play-queue) events on the
	// same connection and returns the next payload carrying an "invites" key.
	readInvitesEvent := func() map[string]any {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read event: %v", err)
			}
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &payload); err != nil {
				t.Fatalf("unmarshal event: %v", err)
			}
			if _, ok := payload["invites"]; ok {
				return payload
			}
		}
	}

	initial := readInvitesEvent()
	if invites, _ := initial["invites"].([]any); len(invites) != 0 {
		t.Fatalf("expected an empty initial snapshot, got %+v", initial)
	}

	if st := doStatus(t, srv, http.MethodPost, "/jam/"+id+"/invites", alice, map[string]any{"username": "bob"}); st != http.StatusNoContent {
		t.Fatalf("invite: expected 204, got %d", st)
	}

	pushed := readInvitesEvent()
	invites, _ := pushed["invites"].([]any)
	if len(invites) != 1 {
		t.Fatalf("expected 1 pushed invite, got %+v", pushed)
	}
	if inv := invites[0].(map[string]any); inv["inviterUsername"] != "alice" {
		t.Fatalf("unexpected invite: %+v", inv)
	}
}
