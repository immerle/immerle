package immerle

import (
	"net/http"
	"testing"
)

func TestJamDeleteHostOnly(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")

	// Alice hosts a session.
	st, body := doMap(t, srv, http.MethodPost, "/jam", alice, map[string]any{"name": "party", "trackIds": []string{"t1"}})
	if st != http.StatusCreated {
		t.Fatalf("create jam: status %d body %+v", st, body)
	}
	session, _ := body["session"].(map[string]any)
	id, _ := session["id"].(string)
	if id == "" {
		t.Fatalf("no jam id in %+v", body)
	}

	// A non-host cannot end it.
	if st := doStatus(t, srv, http.MethodDelete, "/jam/"+id, bob, nil); st != http.StatusForbidden {
		t.Fatalf("non-host delete: expected 403, got %d", st)
	}

	// The host ends it.
	if st := doStatus(t, srv, http.MethodDelete, "/jam/"+id, alice, nil); st != http.StatusNoContent {
		t.Fatalf("host delete: status %d", st)
	}

	// It is gone.
	if st := doStatus(t, srv, http.MethodGet, "/jam/"+id, alice, nil); st != http.StatusNotFound {
		t.Fatalf("get deleted: expected 404, got %d", st)
	}
}
