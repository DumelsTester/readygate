// Package rules defines readiness checklists and reports structural problems.
//
// A checklist is an ordered list of checks. A check passes later evaluation
// only when its prerequisites have passed and its evidence requirements are
// met. Validate does not look at evidence and does not read the clock. It
// reports empty checklists, broken identifiers, unknown prerequisites,
// prerequisite cycles, and ill-formed requirements.
package rules

import "fmt"

// Checklist is a named set of readiness checks.
// Check IDs are case-sensitive and must be unique.
type Checklist struct {
	ID     string
	Name   string
	Checks []Check
}

// Check is one readiness question.
//
// Prerequisites name other checks in the same checklist that must pass
// before this check can pass. Evidence names the kinds of evidence that
// must be valid at evaluation time. A check needs at least one prerequisite
// or one evidence requirement.
type Check struct {
	ID            string
	Title         string
	Prerequisites []string
	Evidence      []Requirement
}

// Requirement is a demand for evidence of one kind.
//
// MinCount is how many valid items are required. A value below 1 is treated
// as 1 by Need. A negative MinCount is reported by Validate and does not
// describe a real requirement.
type Requirement struct {
	Kind     string
	MinCount int
}

// Need returns how many valid evidence items this requirement demands.
// Counts below 1, including the zero value, demand one item.
func (r Requirement) Need() int {
	if r.MinCount < 1 {
		return 1
	}
	return r.MinCount
}

// Validate checks list for structural problems.
// The result is also available as Checklist.Validate.
func Validate(list Checklist) Validation {
	return list.Validate()
}

// ProblemCode identifies a structural checklist problem.
//
// The codes are empty_checklist, empty_id, duplicate_id, no_criteria,
// invalid_requirement, duplicate_prerequisite, unknown_prerequisite, and cycle.
type ProblemCode string

const (
	ProblemEmptyChecklist        ProblemCode = "empty_checklist"
	ProblemEmptyID               ProblemCode = "empty_id"
	ProblemDuplicateID           ProblemCode = "duplicate_id"
	ProblemNoCriteria            ProblemCode = "no_criteria"
	ProblemInvalidRequirement    ProblemCode = "invalid_requirement"
	ProblemDuplicatePrerequisite ProblemCode = "duplicate_prerequisite"
	ProblemUnknownPrerequisite   ProblemCode = "unknown_prerequisite"
	ProblemCycle                 ProblemCode = "cycle"
)

// Problem is one structural finding.
// CheckID is the check that has the problem, when there is one.
// Ref is a related prerequisite ID or evidence kind, when there is one.
type Problem struct {
	Code    ProblemCode
	CheckID string
	Ref     string
	Message string
}

// Validation is the structural result for a checklist.
//
// Ambiguous is true when the checklist has no checks, an empty check ID,
// or a duplicate check ID. Cycle and prerequisite structure is undefined
// in that case, Cycles is nil, and evaluation does not score individual
// checks.
//
// Cycles maps a check ID to one prerequisite cycle that contains it.
// The path does not repeat its first ID at the end. It starts at the
// lexicographically smallest ID in that cycle. When a check sits in more
// than one cycle, the path is the first one found by depth-first search
// in prerequisite list order. Cycles is nil when no check is cyclic.
type Validation struct {
	Problems  []Problem
	Ambiguous bool
	Cycles    map[string][]string
}

// InCycle reports whether Validate found id on a prerequisite cycle.
func (v Validation) InCycle(id string) bool {
	_, ok := v.Cycles[id]
	return ok
}

// FormatCycle renders a cycle path for display, repeating the first ID
// at the end. An empty path renders as an empty string.
func FormatCycle(path []string) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, len(path)+1)
	copy(parts, path)
	parts[len(path)] = path[0]
	return joinArrow(parts)
}

func joinArrow(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += " -> " + p
	}
	return out
}

// String renders a problem on one line.
func (p Problem) String() string {
	if p.CheckID == "" {
		return fmt.Sprintf("%s: %s", p.Code, p.Message)
	}
	return fmt.Sprintf("%s: %s: %s", p.Code, p.CheckID, p.Message)
}
