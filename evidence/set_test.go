package evidence_test

import (
	"errors"
	"testing"
	"time"

	"github.com/DumelsTester/readygate/evidence"
)

func TestItemStateAt(t *testing.T) {
	t.Parallel()
	observed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	item := evidence.Item{ID: "e1", CheckID: "tests", Kind: "test-report", ObservedAt: observed, ExpiresAt: expires}

	tests := []struct {
		name string
		at   time.Time
		item evidence.Item
		want evidence.State
	}{
		{name: "zero evaluation time", at: time.Time{}, item: item, want: evidence.StateNotYetValid},
		{name: "zero observed", at: expires, item: evidence.Item{ExpiresAt: expires}, want: evidence.StateNotYetValid},
		{name: "before observed", at: observed.Add(-time.Nanosecond), item: item, want: evidence.StateNotYetValid},
		{name: "at observed", at: observed, item: item, want: evidence.StateValid},
		{name: "just before expiry", at: expires.Add(-time.Nanosecond), item: item, want: evidence.StateValid},
		{name: "at expiry", at: expires, item: item, want: evidence.StateExpired},
		{name: "after expiry", at: expires.Add(time.Hour), item: item, want: evidence.StateExpired},
		{
			name: "open ended stays valid",
			at:   expires.Add(365 * 24 * time.Hour),
			item: evidence.Item{ObservedAt: observed},
			want: evidence.StateValid,
		},
		{
			name: "future observation wins over a past expiry",
			at:   observed,
			item: evidence.Item{ObservedAt: observed.Add(time.Hour), ExpiresAt: observed},
			want: evidence.StateNotYetValid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.StateAt(tt.at); got != tt.want {
				t.Fatalf("StateAt = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSetAddAndLookup(t *testing.T) {
	t.Parallel()
	observed := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expires := observed.Add(time.Hour)
	valid := evidence.Item{
		ID: "b", CheckID: "tests", Kind: "test-report", Source: "ci", Note: "run 14",
		ObservedAt: observed, ExpiresAt: expires,
	}

	var nilSet *evidence.Set
	if err := nilSet.Add(valid); !errors.Is(err, evidence.ErrInvalidItem) {
		t.Fatalf("nil Add err = %v", err)
	}
	if nilSet.Len() != 0 || nilSet.Matching("tests", "test-report") != nil || nilSet.Items() != nil {
		t.Fatal("nil set lookup was not empty")
	}

	set := evidence.New()
	cases := []evidence.Item{
		{},
		{ID: "x", CheckID: "tests", Kind: "test-report"},
		{ID: "x", CheckID: "tests", Kind: "test-report", ObservedAt: observed, ExpiresAt: observed},
		{ID: "x", CheckID: "tests", Kind: "test-report", ObservedAt: observed, ExpiresAt: observed.Add(-time.Nanosecond)},
	}
	for _, item := range cases {
		if err := set.Add(item); !errors.Is(err, evidence.ErrInvalidItem) {
			t.Fatalf("Add(%+v) err = %v, want invalid", item, err)
		}
	}
	if err := set.Add(valid); err != nil {
		t.Fatal(err)
	}
	if err := set.Add(valid); !errors.Is(err, evidence.ErrDuplicateItem) {
		t.Fatalf("duplicate err = %v", err)
	}
	later := valid
	later.ID = "a"
	later.Note = "other"
	if err := set.Add(later); err != nil {
		t.Fatal(err)
	}
	other := valid
	other.ID = "c"
	other.Kind = "sign-off"
	if err := set.Add(other); err != nil {
		t.Fatal(err)
	}

	if set.Len() != 3 {
		t.Fatalf("len = %d", set.Len())
	}
	matched := set.Matching("tests", "test-report")
	if len(matched) != 2 || matched[0].ID != "a" || matched[1].ID != "b" {
		t.Fatalf("matching = %+v", matched)
	}
	matched[0].Note = "mutated"
	if set.Matching("tests", "test-report")[0].Note != "other" {
		t.Fatal("Matching returned a live alias")
	}
	items := set.Items()
	if len(items) != 3 || items[0].ID != "a" || items[1].ID != "b" || items[2].ID != "c" {
		t.Fatalf("items = %+v", items)
	}
	expiry, ok := evidence.EarliestExpiry(items)
	if !ok || !expiry.Equal(expires) {
		t.Fatalf("earliest expiry = %v ok=%v", expiry, ok)
	}
	if _, ok := evidence.EarliestExpiry(nil); ok {
		t.Fatal("empty expiry should be absent")
	}
	open := []evidence.Item{{ID: "z", ObservedAt: observed}}
	if _, ok := evidence.EarliestExpiry(open); ok {
		t.Fatal("open-ended item has no expiry")
	}
}
