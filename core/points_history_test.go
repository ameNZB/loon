package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// PointsService let a plugin move points but never read them, so a plugin that
// wanted to show a user their own transactions had no way to — the ledger UI
// was stuck on the host. History closes that; these pin its contract.

func TestPointsHistoryNotWiredIsLoud(t *testing.T) {
	// A nil callback must fail loudly rather than look like an empty ledger:
	// "you have no transactions" is a plausible lie that would ship.
	p := NewPoints(PointsAdapter{})
	entries, total, err := p.History(context.Background(), 1, 10, 0)
	if !errors.Is(err, ErrPointsNotWired) {
		t.Errorf("History with no adapter = %v, want ErrPointsNotWired", err)
	}
	if entries != nil || total != 0 {
		t.Errorf("History returned data from an unwired adapter: %v, %d", entries, total)
	}
}

func TestPointsHistoryClampsPaging(t *testing.T) {
	var gotLimit, gotOffset int
	p := NewPoints(PointsAdapter{
		HistoryFn: func(_ context.Context, _ int64, limit, offset int) ([]LedgerEntry, int, error) {
			gotLimit, gotOffset = limit, offset
			return nil, 0, nil
		},
	})

	// An unbounded caller must not turn into an unbounded query: a real ledger
	// runs to tens of thousands of rows.
	if _, _, err := p.History(context.Background(), 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if gotLimit != defaultHistoryLimit {
		t.Errorf("limit 0 passed through as %d, want clamp to %d", gotLimit, defaultHistoryLimit)
	}

	if _, _, err := p.History(context.Background(), 1, 20, -5); err != nil {
		t.Fatal(err)
	}
	if gotLimit != 20 || gotOffset != 0 {
		t.Errorf("History(20, -5) -> limit=%d offset=%d, want 20/0", gotLimit, gotOffset)
	}
}

func TestPointsHistoryPassesThrough(t *testing.T) {
	ref := int64(42)
	want := []LedgerEntry{{
		Amount: -100, Balance: 900, Type: "spend_store_purchase",
		Description: "Bought \"Invite\" from the store", ReferenceID: &ref,
		CreatedAt: time.Now(),
	}}
	p := NewPoints(PointsAdapter{
		HistoryFn: func(_ context.Context, userID int64, _, _ int) ([]LedgerEntry, int, error) {
			if userID != 7 {
				t.Errorf("userID = %d, want 7", userID)
			}
			return want, 31, nil
		},
	})
	got, total, err := p.History(context.Background(), 7, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 31 {
		t.Errorf("total = %d, want 31 (the count must ignore limit/offset so a UI can page)", total)
	}
	if len(got) != 1 || got[0].Amount != -100 || got[0].Balance != 900 || got[0].ReferenceID == nil || *got[0].ReferenceID != 42 {
		t.Errorf("entry mangled in transit: %+v", got)
	}
}
