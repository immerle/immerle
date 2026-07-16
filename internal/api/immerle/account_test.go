package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/testutil"
)

func TestAccountSelfEdit(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "bob", "bobpw", "", "Bob M", false); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{Auth: auth, Users: store.Users, Logger: testutil.NewLogger()})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	bob := login(t, srv, "bob")

	status, u := doMap(t, srv, http.MethodGet, "/me", bob, nil)
	if status != http.StatusOK || u["username"] != "bob" || u["displayName"] != "Bob M" {
		t.Fatalf("account GET wrong: status %d %+v", status, u)
	}

	// Display name is trimmed.
	status, u = doMap(t, srv, http.MethodPatch, "/me", bob, map[string]any{
		"displayName": "  New Bob  ",
		"email":       "bob@example.com",
	})
	if status != http.StatusOK || u["displayName"] != "New Bob" || u["email"] != "bob@example.com" {
		t.Fatalf("account update wrong: status %d %+v", status, u)
	}

	stored, _ := store.Users.GetByUsername(ctx, "bob")
	if stored.DisplayName != "New Bob" || stored.Email != "bob@example.com" {
		t.Fatalf("account not persisted: %+v", stored)
	}

	if code := doStatus(t, srv, http.MethodPatch, "/me", bob, map[string]any{"email": "not-an-email"}); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d", code)
	}

	// Partial update leaves other fields untouched.
	status, u = doMap(t, srv, http.MethodPatch, "/me", bob, map[string]any{"displayName": "Bobby"})
	if status != http.StatusOK || u["displayName"] != "Bobby" || u["email"] != "bob@example.com" {
		t.Fatalf("partial update clobbered email: status %d %+v", status, u)
	}
}
