package lyrics

import "testing"

func TestParseSynced(t *testing.T) {
	doc := Parse("[00:12.50]Hello\n[01:05]World")
	if !doc.Synced {
		t.Fatal("expected synced=true")
	}
	if len(doc.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(doc.Lines))
	}
	if doc.Lines[0].StartMs != 12500 || doc.Lines[0].Text != "Hello" {
		t.Errorf("line0 = %+v, want {12500 Hello}", doc.Lines[0])
	}
	if doc.Lines[1].StartMs != 65000 || doc.Lines[1].Text != "World" {
		t.Errorf("line1 = %+v, want {65000 World}", doc.Lines[1])
	}
}

func TestParsePlain(t *testing.T) {
	doc := Parse("just\ntext")
	if doc.Synced {
		t.Error("expected synced=false")
	}
	if len(doc.Lines) != 2 || doc.Lines[0].Text != "just" {
		t.Errorf("lines = %+v", doc.Lines)
	}
}

func TestParseEmpty(t *testing.T) {
	if doc := Parse("   \n  "); doc.Synced || len(doc.Lines) != 0 {
		t.Errorf("empty input should yield empty doc, got %+v", doc)
	}
}
