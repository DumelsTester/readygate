package evidence

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrInvalidItem is returned when an item is missing an identity, a check,
// a kind, or an observed time, or when its expiry is not after ObservedAt.
var ErrInvalidItem = errors.New("evidence: invalid item")

// ErrDuplicateItem is returned when an item ID is already stored.
var ErrDuplicateItem = errors.New("evidence: duplicate item")

// Set is an in-memory evidence collection keyed by item ID.
//
// Add must not run concurrently with Add, Matching, Items, or Len.
// Concurrent calls to Matching, Items, and Len are safe when Add is not
// running. Evaluation only reads the set.
type Set struct {
	byID map[string]Item
}

// New returns an empty set.
func New() *Set {
	return &Set{byID: make(map[string]Item)}
}

// Len returns the number of stored items. A nil set has length zero.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.byID)
}

// Add stores a copy of it.
// ID, CheckID, Kind, and ObservedAt are required.
// A non-zero ExpiresAt must be after ObservedAt.
func (s *Set) Add(it Item) error {
	if s == nil {
		return fmt.Errorf("%w: nil set", ErrInvalidItem)
	}
	if it.ID == "" || it.CheckID == "" || it.Kind == "" {
		return fmt.Errorf("%w: id, check id, and kind are required", ErrInvalidItem)
	}
	if it.ObservedAt.IsZero() {
		return fmt.Errorf("%w: observed time is required", ErrInvalidItem)
	}
	if !it.ExpiresAt.IsZero() && !it.ExpiresAt.After(it.ObservedAt) {
		return fmt.Errorf("%w: expiry must be after observed time", ErrInvalidItem)
	}
	if s.byID == nil {
		s.byID = make(map[string]Item)
	}
	if _, exists := s.byID[it.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateItem, it.ID)
	}
	s.byID[it.ID] = it
	return nil
}

// Matching returns copies of the items for checkID and kind, sorted by ID.
// A nil set returns nil.
func (s *Set) Matching(checkID, kind string) []Item {
	if s == nil {
		return nil
	}
	var out []Item
	for _, it := range s.byID {
		if it.CheckID == checkID && it.Kind == kind {
			out = append(out, it)
		}
	}
	sortItems(out)
	return out
}

// Items returns a copy of every item, sorted by ID.
// A nil set returns nil.
func (s *Set) Items() []Item {
	if s == nil {
		return nil
	}
	out := make([]Item, 0, len(s.byID))
	for _, it := range s.byID {
		out = append(out, it)
	}
	sortItems(out)
	return out
}

func sortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
}

// EarliestExpiry returns the earliest non-zero expiry among items, and
// whether one exists. It is a convenience for callers inspecting a slice
// returned by Matching or Items.
func EarliestExpiry(items []Item) (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, it := range items {
		if it.ExpiresAt.IsZero() {
			continue
		}
		if !found || it.ExpiresAt.Before(earliest) {
			earliest = it.ExpiresAt
			found = true
		}
	}
	return earliest, found
}
