package immerle

import (
	"net/http"
	"testing"
)

func TestAdminUserManagement(t *testing.T) {
	srv, adminToken, _ := newBrowseEnv(t)

	// Only the seeded admin exists.
	var list struct {
		Users []adminUserView `json:"users"`
	}
	if st := getJSON(t, srv, adminToken, "/admin/users", &list); st != http.StatusOK {
		t.Fatalf("list users: status %d", st)
	}
	if len(list.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(list.Users))
	}

	// Create a non-admin user.
	var created adminUserView
	resp := do(t, srv, http.MethodPost, "/admin/users", adminToken, map[string]any{
		"username": "bob", "password": "bobpw", "email": "bob@x.dev",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create user: status %d", resp.StatusCode)
	}
	decode(t, resp, &created)
	if created.Username != "bob" || created.Admin {
		t.Fatalf("created user: %+v", created)
	}

	// A non-admin may not list users.
	bobToken := login(t, srv, "bob")
	if st := doStatus(t, srv, http.MethodGet, "/admin/users", bobToken, nil); st != http.StatusForbidden {
		t.Fatalf("non-admin list: expected 403, got %d", st)
	}

	// Update: rename and promote to admin.
	if st := doStatus(t, srv, http.MethodPatch, "/admin/users/bob", adminToken, map[string]any{
		"displayName": "Bob M", "admin": true,
	}); st != http.StatusNoContent {
		t.Fatalf("update user: status %d", st)
	}
	var got adminUserView
	if st := getJSON(t, srv, adminToken, "/admin/users/bob", &got); st != http.StatusOK {
		t.Fatalf("get user: status %d", st)
	}
	if got.DisplayName != "Bob M" || !got.Admin {
		t.Fatalf("update not applied: %+v", got)
	}

	// Delete, then it is gone.
	if st := doStatus(t, srv, http.MethodDelete, "/admin/users/bob", adminToken, nil); st != http.StatusNoContent {
		t.Fatalf("delete user: status %d", st)
	}
	if st := doStatus(t, srv, http.MethodGet, "/admin/users/bob", adminToken, nil); st != http.StatusNotFound {
		t.Fatalf("get deleted: expected 404, got %d", st)
	}

	// Self password change.
	if st := doStatus(t, srv, http.MethodPut, "/me/password", adminToken, map[string]any{"password": "newpw"}); st != http.StatusNoContent {
		t.Fatalf("change password: status %d", st)
	}
}
