// Package evidence stores time-bounded facts that support readiness checks.
//
// An item is valid at time T when ObservedAt is set, T is not before
// ObservedAt, and T is before ExpiresAt. The valid interval is half-open:
// [ObservedAt, ExpiresAt). A zero ExpiresAt means the item does not expire.
// An item whose ObservedAt is after T is not yet valid, which is distinct
// from missing evidence and from expired evidence.
//
// This package does not read the clock. Callers choose T and pass it to
// evaluation, so the same items and the same T always classify the same way.
package evidence

import "time"

// State is an item's classification at a single instant.
type State string

const (
	// StateValid means the item counts toward a requirement at that instant.
	StateValid State = "valid"
	// StateExpired means the instant is at or after ExpiresAt.
	StateExpired State = "expired"
	// StateNotYetValid means the item was observed after the instant,
	// or ObservedAt is unset.
	StateNotYetValid State = "not_yet_valid"
)

// Item is one piece of evidence for one check and one kind.
//
// ID is unique inside a Set. CheckID and Kind select which requirement the
// item can satisfy. Source and Note are explanatory labels. Note is kept for
// callers and for the text report; it is not interpreted.
type Item struct {
	ID         string
	CheckID    string
	Kind       string
	Source     string
	Note       string
	ObservedAt time.Time
	ExpiresAt  time.Time
}

// StateAt classifies the item at instant at.
// A zero at, or a zero ObservedAt, is StateNotYetValid.
// When both a future ObservedAt and an earlier ExpiresAt are set, the
// result is StateNotYetValid.
func (it Item) StateAt(at time.Time) State {
	if at.IsZero() || it.ObservedAt.IsZero() || it.ObservedAt.After(at) {
		return StateNotYetValid
	}
	if !it.ExpiresAt.IsZero() && !it.ExpiresAt.After(at) {
		return StateExpired
	}
	return StateValid
}
