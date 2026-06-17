package immerle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// newSetupEnv builds a handler on an empty store (no users) with setup wired.
func newSetupEnv(t *testing.T, requireToken bool) (*httptest.Server, *core.SetupService, *persistence.Store) {
	store := testutil.NewStore(t)
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	setupSvc, err := core.NewSetupService(store.Users, auth, requireToken)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{
		Auth:   auth,
		Users:  store.Users,
		Setup:  setupSvc,
		Logger: testutil.NewLogger(),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, setupSvc, store
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func postJSON(t *testing.T, url string, body map[string]any) (int, map[string]any) {
	t.Helper()
	buf, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSetupStatusThenInitThenLock(t *testing.T) {
	srv, _, _ := newSetupEnv(t, false)

	// Initially needs setup.
	st := getJSON(t, srv.URL+"/setup/status")
	if st["needsSetup"] != true || st["setupTokenRequired"] != false {
		t.Fatalf("unexpected status: %+v", st)
	}

	// Create the first admin.
	code, body := postJSON(t, srv.URL+"/setup/init", map[string]any{
		"username": "kilian", "password": "password123", "email": "k@example.com",
	})
	if code != http.StatusCreated || body["ok"] != true {
		t.Fatalf("init failed: %d %+v", code, body)
	}
	user, _ := body["user"].(map[string]any)
	if user["username"] != "kilian" || user["isAdmin"] != true {
		t.Fatalf("unexpected user payload: %+v", user)
	}

	// Now initialized.
	st = getJSON(t, srv.URL+"/setup/status")
	if st["needsSetup"] != false || st["initialized"] != true {
		t.Fatalf("should be initialized: %+v", st)
	}

	// Locked: a second init is 409.
	code, body = postJSON(t, srv.URL+"/setup/init", map[string]any{
		"username": "evil", "password": "password123",
	})
	if code != http.StatusConflict || body["error"] != "already_initialized" {
		t.Fatalf("expected 409 already_initialized, got %d %+v", code, body)
	}

	// Capabilities reflect setup no longer needed.
	caps := getJSON(t, srv.URL+"/capabilities")
	c, _ := caps["capabilities"].(map[string]any)
	setup, _ := c["setup"].(map[string]any)
	if setup["needed"] != false {
		t.Fatalf("capabilities setup.needed should be false: %+v", setup)
	}
}

func TestSetupValidationOverHTTP(t *testing.T) {
	srv, _, _ := newSetupEnv(t, false)
	code, body := postJSON(t, srv.URL+"/setup/init", map[string]any{
		"username": "bad name!", "password": "short",
	})
	if code != http.StatusBadRequest || body["error"] != "validation" {
		t.Fatalf("expected 400 validation, got %d %+v", code, body)
	}
	details, _ := body["details"].([]any)
	if len(details) != 2 {
		t.Fatalf("expected 2 field errors, got %+v", body["details"])
	}
}

func TestSetupTokenGateOverHTTP(t *testing.T) {
	srv, setupSvc, _ := newSetupEnv(t, true)

	st := getJSON(t, srv.URL+"/setup/status")
	if st["setupTokenRequired"] != true {
		t.Fatalf("token should be required: %+v", st)
	}

	// Missing token → 401.
	code, body := postJSON(t, srv.URL+"/setup/init", map[string]any{
		"username": "kilian", "password": "password123",
	})
	if code != http.StatusUnauthorized || body["error"] != "invalid_setup_token" {
		t.Fatalf("expected 401, got %d %+v", code, body)
	}

	// Correct token → 201.
	code, body = postJSON(t, srv.URL+"/setup/init", map[string]any{
		"username": "kilian", "password": "password123", "setupToken": setupSvc.Token(),
	})
	if code != http.StatusCreated || body["ok"] != true {
		t.Fatalf("expected 201 with correct token, got %d %+v", code, body)
	}
}
