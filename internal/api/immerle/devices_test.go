package immerle

import (
	"net/http"
	"testing"
)

func TestDevicesListShowsConnectedAndRevoke(t *testing.T) {
	srv, _ := newEnv(t)
	token := login(t, srv, "alice")

	// The login itself, plus this very request, are authenticated calls that
	// touch last-seen — the device should show as connected without waiting
	// for a flush (see DeviceRepo.withPending).
	status, devices := doArr(t, srv, http.MethodGet, "/devices", token, nil)
	if status != http.StatusOK {
		t.Fatalf("devices status %d", status)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %+v", devices)
	}
	d := devices[0].(map[string]any)
	if d["connected"] != true {
		t.Fatalf("expected the just-used device to be connected: %+v", d)
	}
	id, _ := d["id"].(string)
	if id == "" {
		t.Fatalf("device missing id: %+v", d)
	}

	if code := doStatus(t, srv, http.MethodDelete, "/devices/"+id, token, nil); code != http.StatusNoContent {
		t.Fatalf("revoke status %d", code)
	}

	// Revoked devices are excluded from the list, but the token itself is now
	// invalid too (its device row is revoked) — so re-list with a fresh login.
	token2 := login(t, srv, "alice")
	_, devices = doArr(t, srv, http.MethodGet, "/devices", token2, nil)
	if len(devices) != 1 {
		t.Fatalf("expected only the new session to be listed, got %+v", devices)
	}
	if devices[0].(map[string]any)["id"] == id {
		t.Fatalf("revoked device should not be listed: %+v", devices)
	}
}
