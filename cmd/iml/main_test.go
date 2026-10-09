package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func press(m model, keys ...string) model {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

func TestSearchBarTakesEveryLetterWhileTyping(t *testing.T) {
	m := model{typing: true, queue: []Song{{ID: "1"}}, client: &Client{}}
	m = press(m, "q", "n", "r", "s")
	if m.query != "qnrs" || m.repeat != repeatOff || m.shuffle {
		t.Fatalf("typing mode: query=%q repeat=%v shuffle=%v", m.query, m.repeat, m.shuffle)
	}

	m = press(m, "esc", "r", "/", "x")
	if m.repeat == repeatOff || m.query != "qnrsx" || !m.typing {
		t.Fatalf("controls then search: query=%q repeat=%v typing=%v", m.query, m.repeat, m.typing)
	}
}

func TestMatchesLiked(t *testing.T) {
	for _, q := range []string{"j'aime", "J'ai", "liked", "fav"} {
		if !matchesLiked(q) {
			t.Errorf("%q should match the liked songs", q)
		}
	}
	if matchesLiked("rap fr") {
		t.Error("rap fr should not match the liked songs")
	}
}
