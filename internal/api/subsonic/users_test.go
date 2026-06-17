package subsonic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gossignol/gossignol/internal/core"
	"github.com/gossignol/gossignol/internal/testutil"
)

// TestGetUserReturnsDisplayName verifies getUser surfaces the display name and
// that updateUser can change it. No ffmpeg needed — pure user-management path.
func TestGetUserReturnsDisplayName(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, err := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := auth.CreateUser(ctx, "admin", "pw", "", "Big Boss", true)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Deps{Auth: auth, Users: store.Users}}

	getUser := func(username string) User {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/rest/getUser?f=json&username="+username, nil)
		_ = r.ParseForm()
		r = r.WithContext(context.WithValue(r.Context(), userKey, admin))
		w := httptest.NewRecorder()
		h.handleGetUser(w, r)
		var env jsonResponse
		if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if env.Response.User == nil {
			t.Fatalf("no user in response: %s", w.Body.String())
		}
		return *env.Response.User
	}

	if u := getUser("admin"); u.DisplayName != "Big Boss" {
		t.Fatalf("getUser displayName = %q, want %q", u.DisplayName, "Big Boss")
	}

	// updateUser changes the display name.
	form := url.Values{"username": {"admin"}, "displayName": {"  New Name  "}}
	ur := httptest.NewRequest(http.MethodGet, "/rest/updateUser?"+form.Encode(), nil)
	_ = ur.ParseForm()
	ur = ur.WithContext(context.WithValue(ur.Context(), userKey, admin))
	h.handleUpdateUser(httptest.NewRecorder(), ur)

	if u := getUser("admin"); u.DisplayName != "New Name" {
		t.Fatalf("after updateUser displayName = %q, want %q", u.DisplayName, "New Name")
	}
}
