package immerle

import (
	"reflect"
	"testing"

	"github.com/immerle/immerle/internal/models"
)

// The PATCH guard writes exactly what settingsPatchRequest declares: writable
// fields take the decoded value, everything else (hub identities, private
// keys, import credentials, toggles with their own endpoint) keeps its value.
func TestCopyDeclaredWritesOnlyDeclaredFields(t *testing.T) {
	current := models.DefaultRuntimeSettings()
	current.Federation.InstanceID = "inst-1"
	current.Federation.PrivateKey = "fed-key"
	current.Discover.PrivateKey = "disc-key"
	current.Import.Sources = map[string]map[string]string{"spotify": {"clientSecret": "s"}}

	merged := current
	merged.Sonic.Workers = current.Sonic.Workers + 3
	merged.Providers.SearchTimeoutSeconds = 42
	merged.Server.CORSAllowedOrigins = []string{"https://a.example"}
	merged.Discover.Enabled = !current.Discover.Enabled
	merged.Federation.SyncPlaylists = !current.Federation.SyncPlaylists
	// Not writable through this endpoint:
	merged.Federation.InstanceID = "hijacked"
	merged.Federation.PrivateKey = "hijacked"
	merged.Discover.PrivateKey = "hijacked"
	merged.Discover.InstanceID = "hijacked"
	merged.Import.Sources = nil
	merged.SmartPlaylists.Enabled = !current.SmartPlaylists.Enabled

	next := current
	copyDeclared(reflect.TypeOf(settingsPatchRequest{}), reflect.ValueOf(&next).Elem(), reflect.ValueOf(merged))

	if next.Sonic.Workers != merged.Sonic.Workers || next.Providers.SearchTimeoutSeconds != 42 ||
		len(next.Server.CORSAllowedOrigins) != 1 || next.Discover.Enabled != merged.Discover.Enabled ||
		next.Federation.SyncPlaylists != merged.Federation.SyncPlaylists {
		t.Fatalf("writable fields not applied: %+v", next)
	}
	if next.Federation.InstanceID != "inst-1" || next.Federation.PrivateKey != "fed-key" ||
		next.Discover.PrivateKey != "disc-key" || next.Discover.InstanceID != current.Discover.InstanceID ||
		next.Import.Sources == nil || next.SmartPlaylists.Enabled != current.SmartPlaylists.Enabled {
		t.Fatalf("a non-writable field was overwritten: %+v", next)
	}
}
