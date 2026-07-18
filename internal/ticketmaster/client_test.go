package ticketmaster

import "testing"

func TestFilterByCityLooseSubstringMatch(t *testing.T) {
	events := []Event{
		{ID: "1", City: "Paris"},
		{ID: "2", City: "Paris La Défense"}, // suburb venue, still "Paris" enough
		{ID: "3", City: "London"},
		{ID: "4", City: ""}, // unknown city, can't verify — must be dropped
	}
	got := filterByCity(events, "Paris", 10)
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("filterByCity(Paris) = %+v, want events 1 and 2 only", got)
	}
}

func TestFilterByCityRespectsLimit(t *testing.T) {
	events := []Event{{ID: "1", City: "Paris"}, {ID: "2", City: "Paris"}, {ID: "3", City: "Paris"}}
	got := filterByCity(events, "Paris", 2)
	if len(got) != 2 {
		t.Fatalf("filterByCity(limit=2) = %d events, want 2", len(got))
	}
}

func TestFilterByCityNoMatch(t *testing.T) {
	events := []Event{{ID: "1", City: "London"}, {ID: "2", City: "Berlin"}}
	got := filterByCity(events, "Paris", 10)
	if len(got) != 0 {
		t.Fatalf("filterByCity(no match) = %+v, want empty", got)
	}
}
