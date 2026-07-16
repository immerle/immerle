package immerle

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"

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
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, setupSvc, store
}

func TestSetupStatusThenInitThenLock(t *testing.T) {
	srv, _, _ := newSetupEnv(t, false)

	// Initially needs setup.
	_, st := doMap(t, srv, http.MethodGet, "/setup", "", nil)
	if st["needsSetup"] != true || st["setupTokenRequired"] != false {
		t.Fatalf("unexpected status: %+v", st)
	}

	code, body := doMap(t, srv, http.MethodPost, "/setup", "", map[string]any{
		"username": "kilian", "password": "password123", "email": "k@example.com",
	})
	if code != http.StatusCreated {
		t.Fatalf("init failed: %d %+v", code, body)
	}
	if body["username"] != "kilian" || body["isAdmin"] != true {
		t.Fatalf("unexpected user payload: %+v", body)
	}

	// Now initialized.
	_, st = doMap(t, srv, http.MethodGet, "/setup", "", nil)
	if st["needsSetup"] != false || st["initialized"] != true {
		t.Fatalf("should be initialized: %+v", st)
	}

	// Locked: a second init is 409.
	code, body = doMap(t, srv, http.MethodPost, "/setup", "", map[string]any{
		"username": "evil", "password": "password123",
	})
	errObj, _ := body["error"].(map[string]any)
	if code != http.StatusConflict || errObj["code"] != "already_initialized" {
		t.Fatalf("expected 409 already_initialized, got %d %+v", code, body)
	}

	// Capabilities reflect setup no longer needed.
	_, caps := doMap(t, srv, http.MethodGet, "/capabilities", "", nil)
	c, _ := caps["capabilities"].(map[string]any)
	setup, _ := c["setup"].(map[string]any)
	if setup["needed"] != false {
		t.Fatalf("capabilities setup.needed should be false: %+v", setup)
	}
}

func TestSetupValidationOverHTTP(t *testing.T) {
	srv, _, _ := newSetupEnv(t, false)
	code, body := doMap(t, srv, http.MethodPost, "/setup", "", map[string]any{
		"username": "bad name!", "password": "short",
	})
	errObj, _ := body["error"].(map[string]any)
	if code != http.StatusBadRequest || errObj["code"] != "validation" {
		t.Fatalf("expected 400 validation, got %d %+v", code, body)
	}
	fields, _ := errObj["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("expected 2 field errors, got %+v", errObj["fields"])
	}
}

func TestSetupTokenGateOverHTTP(t *testing.T) {
	srv, setupSvc, _ := newSetupEnv(t, true)

	_, st := doMap(t, srv, http.MethodGet, "/setup", "", nil)
	if st["setupTokenRequired"] != true {
		t.Fatalf("token should be required: %+v", st)
	}

	// Missing token → 401.
	code, body := doMap(t, srv, http.MethodPost, "/setup", "", map[string]any{
		"username": "kilian", "password": "password123",
	})
	errObj, _ := body["error"].(map[string]any)
	if code != http.StatusUnauthorized || errObj["code"] != "invalid_setup_token" {
		t.Fatalf("expected 401, got %d %+v", code, body)
	}

	// Correct token → 201.
	code, body = doMap(t, srv, http.MethodPost, "/setup", "", map[string]any{
		"username": "kilian", "password": "password123", "setupToken": setupSvc.Token(),
	})
	if code != http.StatusCreated {
		t.Fatalf("expected 201 with correct token, got %d %+v", code, body)
	}
}
