package providers

import "testing"

func TestParseConfig(t *testing.T) {
	// Empty / "{}" → zero config.
	for _, s := range []string{"", "  ", "{}"} {
		if c, err := ParseConfig(s); err != nil || c.Header != nil || c.Params != nil {
			t.Fatalf("ParseConfig(%q) = %+v, %v; want zero config", s, c, err)
		}
	}

	// New schema.
	c, err := ParseConfig(`{"header":{"Authorization":"Bearer x"},"params":{"client_id":"abc"},"timeoutSeconds":5}`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Header["Authorization"] != "Bearer x" || c.Params["client_id"] != "abc" || c.TimeoutSeconds != 5 {
		t.Fatalf("unexpected parse: %+v", c)
	}

	// Legacy "headers" alias keeps working.
	c, err = ParseConfig(`{"headers":{"X-Key":"v"}}`)
	if err != nil || c.Header["X-Key"] != "v" {
		t.Fatalf("legacy alias: %+v, %v", c, err)
	}

	// Unknown top-level keys (e.g. the old flat built-in keys) are rejected.
	if _, err := ParseConfig(`{"max_items":"8"}`); err == nil {
		t.Fatal("expected error for unknown key")
	}
	if _, err := ParseConfig(`{bad json`); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConfigParam(t *testing.T) {
	c := Config{Params: map[string]string{"a": "1", "empty": ""}}
	if c.Param("a", "def") != "1" {
		t.Fatal("should return set value")
	}
	if c.Param("empty", "def") != "def" || c.Param("missing", "def") != "def" {
		t.Fatal("empty/missing should fall back to default")
	}
}
