package eval

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DumelsTester/readygate/evidence"
	"github.com/DumelsTester/readygate/rules"
)

// Evaluate scores list against evidence at the instant at.
//
// at is required and is stored on the result in UTC. Expiry uses the
// instant, so the location of at does not change the outcome.
//
// When the checklist is ambiguous, the result carries Problems, Ready is
// false, and Checks is empty.
//
// Otherwise every check is scored. The status is the first match:
//
//  1. no criteria, or an invalid requirement: failed
//  2. the check is on a prerequisite cycle: failed
//  3. a prerequisite ID does not exist: failed
//  4. a prerequisite is not passed: blocked
//  5. an evidence requirement is not met: failed
//  6. passed
//
// A blocked check still lists its evidence reasons. Evidence is not scored
// when that check's requirements are invalid. Reasons, when present, follow
// this order: invalid requirements, cycle, unknown prerequisites,
// prerequisites that are not passed, a summary of prerequisites that passed,
// then evidence requirements in listed order.
//
// A duplicate prerequisite does not change the check status by itself.
// It is a structural problem, so Ready is false, and it is not applied twice.
func Evaluate(list rules.Checklist, ev EvidenceView, at time.Time) (Result, error) {
	if at.IsZero() {
		return Result{}, ErrEvaluationTime
	}
	if ev == nil {
		return Result{}, ErrNilEvidence
	}

	validation := list.Validate()
	result := Result{
		ChecklistID: list.ID,
		Name:        list.Name,
		EvaluatedAt: at.UTC(),
		Problems:    validation.Problems,
	}
	if validation.Ambiguous {
		return result, nil
	}

	scored := scoreAll(list, validation, ev, at)
	result.Checks = make([]CheckResult, 0, len(list.Checks))
	allPassed := true
	for _, check := range list.Checks {
		scoredCheck := scored[check.ID]
		if scoredCheck.Status != StatusPassed {
			allPassed = false
		}
		result.Checks = append(result.Checks, scoredCheck)
	}
	result.Ready = allPassed && len(validation.Problems) == 0
	return result, nil
}

func scoreAll(list rules.Checklist, validation rules.Validation, ev EvidenceView, at time.Time) map[string]CheckResult {
	ids := make(map[string]struct{}, len(list.Checks))
	for _, check := range list.Checks {
		ids[check.ID] = struct{}{}
	}

	dependents := make(map[string][]string)
	remaining := make(map[string]int)
	for _, check := range list.Checks {
		seen := make(map[string]bool)
		for _, prereq := range check.Prerequisites {
			if seen[prereq] {
				continue
			}
			seen[prereq] = true
			if _, ok := ids[prereq]; !ok {
				continue
			}
			// Edges inside a cycle would deadlock a prerequisite walk.
			// The cycle finding already explains those edges.
			if validation.InCycle(check.ID) && validation.InCycle(prereq) {
				continue
			}
			remaining[check.ID]++
			dependents[prereq] = append(dependents[prereq], check.ID)
		}
	}

	scored := make(map[string]CheckResult, len(list.Checks))
	for len(scored) < len(list.Checks) {
		picked := -1
		for i, check := range list.Checks {
			if _, done := scored[check.ID]; done {
				continue
			}
			if remaining[check.ID] == 0 {
				picked = i
				break
			}
		}
		if picked < 0 {
			for _, check := range list.Checks {
				if _, done := scored[check.ID]; done {
					continue
				}
				scored[check.ID] = CheckResult{
					ID:     check.ID,
					Title:  check.Title,
					Status: StatusFailed,
					Reasons: []Reason{{
						Code:    CodeCycle,
						Message: "prerequisite cycle could not be ordered",
					}},
				}
			}
			break
		}
		check := list.Checks[picked]
		scored[check.ID] = scoreCheck(check, validation, ids, scored, ev, at)
		for _, dependent := range dependents[check.ID] {
			remaining[dependent]--
		}
	}
	return scored
}

func scoreCheck(check rules.Check, validation rules.Validation, ids map[string]struct{}, done map[string]CheckResult, ev EvidenceView, at time.Time) CheckResult {
	result := CheckResult{ID: check.ID, Title: check.Title}
	var (
		reasons        []Reason
		invalidReasons []Reason
		unknownReasons []Reason
		noCriteria     Reason
		cycleReason    Reason
		hasNoCriteria  bool
		invalid        bool
		inCycle        bool
	)

	for _, problem := range validation.Problems {
		if problem.CheckID != check.ID {
			continue
		}
		switch problem.Code {
		case rules.ProblemNoCriteria:
			hasNoCriteria = true
			noCriteria = Reason{Code: CodeNoCriteria, Message: problem.Message}
		case rules.ProblemInvalidRequirement:
			invalid = true
			invalidReasons = append(invalidReasons, Reason{
				Code:    CodeInvalidRequirement,
				Kind:    problem.Ref,
				Message: problem.Message,
			})
		case rules.ProblemUnknownPrerequisite:
			unknownReasons = append(unknownReasons, Reason{
				Code:    CodeUnknownPrerequisite,
				CheckID: problem.Ref,
				Message: problem.Message,
			})
		case rules.ProblemCycle:
			inCycle = true
			cycleReason = Reason{Code: CodeCycle, Message: problem.Message}
		}
	}
	if hasNoCriteria {
		result.Status = StatusFailed
		result.Reasons = []Reason{noCriteria}
		return result
	}
	reasons = append(reasons, invalidReasons...)
	if inCycle {
		reasons = append(reasons, cycleReason)
	}
	reasons = append(reasons, unknownReasons...)

	seen := make(map[string]bool)
	var known []string
	for _, prereq := range check.Prerequisites {
		if seen[prereq] {
			continue
		}
		seen[prereq] = true
		if _, ok := ids[prereq]; !ok {
			continue
		}
		known = append(known, prereq)
	}

	var blocked []string
	var passed []string
	for _, prereq := range known {
		if inCycle && validation.InCycle(prereq) {
			continue
		}
		status := done[prereq].Status
		if status == "" {
			status = StatusFailed
		}
		if status == StatusPassed {
			passed = append(passed, prereq)
			continue
		}
		blocked = append(blocked, prereq)
		reasons = append(reasons, Reason{
			Code:    CodePrerequisiteNotPassed,
			CheckID: prereq,
			Message: prereq + " is " + string(status),
		})
	}
	if !inCycle && len(unknownReasons) == 0 && len(blocked) == 0 && len(passed) > 0 && !invalid {
		reasons = append(reasons, Reason{
			Code:    CodePrerequisitesSatisfied,
			Message: strings.Join(passed, ", "),
		})
	}

	evidenceOK := true
	if !invalid && len(check.Evidence) > 0 {
		evidenceReasons, ok := scoreEvidence(check, ev, at)
		reasons = append(reasons, evidenceReasons...)
		evidenceOK = ok
	}

	switch {
	case invalid || inCycle || len(unknownReasons) > 0:
		result.Status = StatusFailed
	case len(blocked) > 0:
		result.Status = StatusBlocked
	case !evidenceOK:
		result.Status = StatusFailed
	default:
		result.Status = StatusPassed
	}
	result.Reasons = reasons
	return result
}

func scoreEvidence(check rules.Check, ev EvidenceView, at time.Time) ([]Reason, bool) {
	var reasons []Reason
	allOK := true
	seen := make(map[string]bool)
	for _, requirement := range check.Evidence {
		if requirement.Kind == "" || requirement.MinCount < 0 || seen[requirement.Kind] {
			continue
		}
		seen[requirement.Kind] = true
		reason, ok := scoreRequirement(check.ID, requirement, ev, at)
		reasons = append(reasons, reason)
		if !ok {
			allOK = false
		}
	}
	return reasons, allOK
}

func scoreRequirement(checkID string, requirement rules.Requirement, ev EvidenceView, at time.Time) (Reason, bool) {
	matched := append([]evidence.Item(nil), ev.Matching(checkID, requirement.Kind)...)
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].ID < matched[j].ID
	})

	var valid, expired, notYet []evidence.Item
	for _, item := range matched {
		switch item.StateAt(at) {
		case evidence.StateValid:
			valid = append(valid, item)
		case evidence.StateExpired:
			expired = append(expired, item)
		default:
			notYet = append(notYet, item)
		}
	}

	need := requirement.Need()
	reason := Reason{
		Kind:        requirement.Kind,
		Need:        need,
		Valid:       len(valid),
		Expired:     len(expired),
		NotYetValid: len(notYet),
	}

	switch {
	case len(valid) >= need:
		reason.Code = CodeEvidenceSatisfied
		reason.EvidenceIDs = itemIDs(valid)
		reason.Message = fmt.Sprintf("kind %q: valid %d, need %d, evidence %s",
			requirement.Kind, len(valid), need, itemLabels(valid))
		return reason, true
	case len(valid) == 0 && len(expired) == 0 && len(notYet) == 0:
		reason.Code = CodeEvidenceMissing
		reason.Message = fmt.Sprintf("kind %q: valid 0, need %d", requirement.Kind, need)
	case len(valid) == 0 && len(expired) > 0 && len(notYet) == 0:
		reason.Code = CodeEvidenceExpired
		reason.EvidenceIDs = itemIDs(expired)
		reason.Message = fmt.Sprintf("kind %q: valid 0, need %d, expired %d, evidence %s",
			requirement.Kind, need, len(expired), itemLabels(expired))
	case len(valid) == 0 && len(notYet) > 0 && len(expired) == 0:
		reason.Code = CodeEvidenceNotYetValid
		reason.EvidenceIDs = itemIDs(notYet)
		reason.Message = fmt.Sprintf("kind %q: valid 0, need %d, not yet valid %d, evidence %s",
			requirement.Kind, need, len(notYet), itemLabels(notYet))
	default:
		reason.Code = CodeEvidenceInsufficient
		reason.EvidenceIDs = itemIDs(valid)
		reason.Message = fmt.Sprintf("kind %q: valid %d, need %d, expired %d, not yet valid %d, valid evidence %s, expired evidence %s, not yet valid evidence %s",
			requirement.Kind, len(valid), need, len(expired), len(notYet),
			itemLabels(valid), itemLabels(expired), itemLabels(notYet))
	}
	return reason, false
}

func itemIDs(items []evidence.Item) []string {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func itemLabels(items []evidence.Item) string {
	if len(items) == 0 {
		return "-"
	}
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = itemLabel(item)
	}
	return strings.Join(labels, ", ")
}

func itemLabel(item evidence.Item) string {
	label := item.ID
	if item.Source != "" {
		label += " [" + item.Source + "]"
	}
	note := oneLine(item.Note)
	if note != "" {
		label += " {" + note + "}"
	}
	return label
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// The in-memory evidence set is the EvidenceView implementation shipped with this module.
var _ EvidenceView = (*evidence.Set)(nil)
