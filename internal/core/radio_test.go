package core

import (
	"context"
	"errors"
	"testing"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestRadioServiceRules(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	svc := NewRadioService(store.Radio)

	if _, err := svc.Create(ctx, models.RadioStation{Name: "Bad", StreamURL: "file:///etc/passwd"}); !errors.Is(err, ErrInvalidStation) {
		t.Fatalf("non-http stream should be rejected, got %v", err)
	}
	st, err := svc.Create(ctx, models.RadioStation{Name: " FIP ", StreamURL: "https://fip.example/live"})
	if err != nil || st.Name != "FIP" {
		t.Fatalf("create: %+v %v", st, err)
	}
	if _, _, err := svc.Update(ctx, st.ID, func(s *models.RadioStation) { s.Name = "" }); !errors.Is(err, ErrInvalidStation) {
		t.Fatalf("clearing the name should be rejected, got %v", err)
	}

	if err := store.Radio.EnsureBuiltins(ctx); err != nil {
		t.Fatal(err)
	}
	all, err := store.Radio.ListForUser(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range all {
		if !b.Builtin {
			continue
		}
		if _, _, err := svc.Update(ctx, b.ID, func(s *models.RadioStation) { s.Name = "x" }); !errors.Is(err, ErrBuiltinStation) {
			t.Fatalf("built-in edit should be refused, got %v", err)
		}
		if err := svc.Delete(ctx, b.ID); !errors.Is(err, ErrBuiltinStation) {
			t.Fatalf("built-in delete should be refused, got %v", err)
		}
		return
	}
	t.Fatal("no built-in station seeded")
}
