package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	creds := func(extra url.Values) url.Values {
		v := url.Values{"u": {"bob"}, "p": {"bobpw"}, "c": {"test"}}
		for k, vals := range extra {
			v[k] = vals
		}
		return v
	}

	// GET own account exposes the (empty) email.
	body := postFormGet(t, srv, "/account", creds(nil))
	u, _ := body["user"].(map[string]any)
	if u == nil || u["username"] != "bob" || u["displayName"] != "Bob M" {
		t.Fatalf("account GET wrong: %+v", body["user"])
	}

	// POST updates display name + email (display name is trimmed).
	body = postForm(t, srv, "/account", creds(url.Values{"displayName": {"  New Bob  "}, "email": {"bob@example.com"}}))
	u, _ = body["user"].(map[string]any)
	if u["displayName"] != "New Bob" || u["email"] != "bob@example.com" {
		t.Fatalf("account update wrong: %+v", body["user"])
	}

	// Persisted across requests.
	stored, _ := store.Users.GetByUsername(ctx, "bob")
	if stored.DisplayName != "New Bob" || stored.Email != "bob@example.com" {
		t.Fatalf("account not persisted: %+v", stored)
	}

	// Invalid email is rejected.
	if code := postFormStatus(t, srv, "/account", creds(url.Values{"email": {"not-an-email"}})); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d", code)
	}

	// A partial update touches only the field provided (email unchanged here).
	body = postForm(t, srv, "/account", creds(url.Values{"displayName": {"Bobby"}}))
	u, _ = body["user"].(map[string]any)
	if u["displayName"] != "Bobby" || u["email"] != "bob@example.com" {
		t.Fatalf("partial update clobbered email: %+v", body["user"])
	}
}
