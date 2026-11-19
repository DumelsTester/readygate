// Package eval scores a checklist against evidence at a caller-supplied time.
//
// Evaluate does not read the wall clock. Pass the same checklist, evidence,
// and time to get the same result. A check passes when its prerequisites
// passed and each evidence requirement has enough valid items at that time.
// Missing, expired, and not-yet-valid evidence are different failures.
// A check whose prerequisite did not pass is blocked, even if its own
// evidence is in good shape, so a handover report can show what is waiting
// on something else.
package eval

import (
	"errors"
	"time"

	"github.com/DumelsTester/readygate/evidence"
	"github.com/DumelsTester/readygate/rules"
)

// ErrEvaluationTime is returned when the evaluation time is the zero time.
var ErrEvaluationTime = errors.New("eval: evaluation time is zero")

// ErrNilEvidence is returned when the evidence view is nil.
var ErrNilEvidence = errors.New("eval: evidence is nil")

// EvidenceView is the read API evaluation needs from an evidence source.
// *evidence.Set implements it. Another source can implement it later
// without changing how checks are scored.
type EvidenceView interface {
	Matching(checkID, kind string) []evidence.Item
}

// Status is the outcome of one check.
type Status string

const (
	// StatusPassed means prerequisites passed and evidence requirements were met.
	StatusPassed Status = "passed"
	// StatusFailed means this check's definition or evidence is unsatisfied.
	// Unknown prerequisites, cycles, invalid requirements, and evidence gaps
	// fail the check itself.
	StatusFailed Status = "failed"
	// StatusBlocked means the check was scored, but a prerequisite is failed
	// or blocked, so this check cannot pass yet.
	StatusBlocked Status = "blocked"
)

// Code is a stable explanation identifier.
type Code string

const (
	CodePrerequisitesSatisfied Code = "prerequisites_satisfied"
	CodePrerequisiteNotPassed  Code = "prerequisite_not_passed"
	CodeUnknownPrerequisite    Code = "unknown_prerequisite"
	CodeCycle                  Code = "cycle"
	CodeNoCriteria             Code = "no_criteria"
	CodeInvalidRequirement     Code = "invalid_requirement"
	CodeEvidenceSatisfied      Code = "evidence_satisfied"
	CodeEvidenceMissing        Code = "evidence_missing"
	CodeEvidenceExpired        Code = "evidence_expired"
	CodeEvidenceNotYetValid    Code = "evidence_not_yet_valid"
	CodeEvidenceInsufficient   Code = "evidence_insufficient"
)

// Reason is one explanation attached to a check.
//
// CheckID is a related check, such as a prerequisite.
// Kind, the counts, and EvidenceIDs describe an evidence requirement.
// EvidenceIDs are sorted item IDs for the group named by Code: valid items
// when the requirement is satisfied or short, expired items when the code
// is evidence_expired, and not-yet-valid items when the code is
// evidence_not_yet_valid. Message carries the other groups.
type Reason struct {
	Code        Code
	Message     string
	CheckID     string
	Kind        string
	Need        int
	Valid       int
	Expired     int
	NotYetValid int
	EvidenceIDs []string
}

// String renders the reason as "<code>: <message>".
func (r Reason) String() string {
	return string(r.Code) + ": " + r.Message
}

// CheckResult is the explanation for one check.
type CheckResult struct {
	ID      string
	Title   string
	Status  Status
	Reasons []Reason
}

// Explain joins the check's reasons, one per line.
// An empty reason list returns an empty string.
func (c CheckResult) Explain() string {
	if len(c.Reasons) == 0 {
		return ""
	}
	out := c.Reasons[0].String()
	for _, reason := range c.Reasons[1:] {
		out += "\n" + reason.String()
	}
	return out
}

// Result is a full evaluation.
//
// EvaluatedAt is the supplied instant in UTC.
// Ready is true only when every check passed and the checklist has no
// structural problems. A passed check can still leave Ready false when
// the checklist itself is ill-formed, for example because a prerequisite
// is duplicated.
//
// Checks follows the checklist order. It is empty when the checklist is
// ambiguous (no checks, empty IDs, or duplicate IDs); Problems explains why.
type Result struct {
	ChecklistID string
	Name        string
	EvaluatedAt time.Time
	Ready       bool
	Problems    []rules.Problem
	Checks      []CheckResult
}

// Find returns the check result for id.
func (r Result) Find(id string) (CheckResult, bool) {
	for _, check := range r.Checks {
		if check.ID == id {
			return check, true
		}
	}
	return CheckResult{}, false
}
