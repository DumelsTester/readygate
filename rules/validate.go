package rules

import "fmt"

// Validate reports structural problems in checklist order.
// It does not read evidence or the clock.
func (c Checklist) Validate() Validation {
	if len(c.Checks) == 0 {
		return Validation{
			Ambiguous: true,
			Problems: []Problem{{
				Code:    ProblemEmptyChecklist,
				Message: "checklist has no checks",
			}},
		}
	}

	problems := make([]Problem, 0)
	index := make(map[string]int, len(c.Checks))
	ambiguous := false
	for i, check := range c.Checks {
		if check.ID == "" {
			ambiguous = true
			problems = append(problems, Problem{
				Code:    ProblemEmptyID,
				Message: fmt.Sprintf("check at index %d has an empty id", i),
			})
			continue
		}
		if _, ok := index[check.ID]; ok {
			ambiguous = true
			problems = append(problems, Problem{
				Code:    ProblemDuplicateID,
				CheckID: check.ID,
				Message: fmt.Sprintf("check id %q is used more than once", check.ID),
			})
			continue
		}
		index[check.ID] = i
	}
	if ambiguous {
		return Validation{Ambiguous: true, Problems: problems}
	}

	edges := make(map[string][]string, len(c.Checks))
	for _, check := range c.Checks {
		problems = append(problems, localProblems(check)...)
		edges[check.ID] = knownPrereqs(check, index, &problems)
	}

	var cycles map[string][]string
	for _, check := range c.Checks {
		if !reachesSelf(check.ID, edges) {
			continue
		}
		path := cyclePath(check.ID, edges)
		if cycles == nil {
			cycles = make(map[string][]string)
		}
		cycles[check.ID] = path
		problems = append(problems, Problem{
			Code:    ProblemCycle,
			CheckID: check.ID,
			Message: "prerequisite cycle " + FormatCycle(path),
		})
	}

	return Validation{Problems: problems, Cycles: cycles}
}

func localProblems(check Check) []Problem {
	if len(check.Prerequisites) == 0 && len(check.Evidence) == 0 {
		return []Problem{{
			Code:    ProblemNoCriteria,
			CheckID: check.ID,
			Message: "check has no prerequisites and no evidence requirements",
		}}
	}
	var problems []Problem
	seenKind := make(map[string]bool)
	for _, req := range check.Evidence {
		switch {
		case req.Kind == "":
			problems = append(problems, Problem{
				Code:    ProblemInvalidRequirement,
				CheckID: check.ID,
				Message: "evidence kind is empty",
			})
		case req.MinCount < 0:
			problems = append(problems, Problem{
				Code:    ProblemInvalidRequirement,
				CheckID: check.ID,
				Ref:     req.Kind,
				Message: fmt.Sprintf("evidence kind %q has a negative minimum count", req.Kind),
			})
		case seenKind[req.Kind]:
			problems = append(problems, Problem{
				Code:    ProblemInvalidRequirement,
				CheckID: check.ID,
				Ref:     req.Kind,
				Message: fmt.Sprintf("evidence kind %q is listed more than once", req.Kind),
			})
		default:
			seenKind[req.Kind] = true
		}
	}
	return problems
}

// knownPrereqs appends duplicate and unknown prerequisite problems and
// returns the unique prerequisite IDs that name a check in index.
func knownPrereqs(check Check, index map[string]int, problems *[]Problem) []string {
	seen := make(map[string]bool)
	var known []string
	for _, id := range check.Prerequisites {
		if seen[id] {
			*problems = append(*problems, Problem{
				Code:    ProblemDuplicatePrerequisite,
				CheckID: check.ID,
				Ref:     id,
				Message: fmt.Sprintf("prerequisite %q is listed more than once", id),
			})
			continue
		}
		seen[id] = true
		if _, ok := index[id]; !ok {
			*problems = append(*problems, Problem{
				Code:    ProblemUnknownPrerequisite,
				CheckID: check.ID,
				Ref:     id,
				Message: fmt.Sprintf("prerequisite %q does not exist", id),
			})
			continue
		}
		known = append(known, id)
	}
	return known
}

func reachesSelf(start string, edges map[string][]string) bool {
	seen := make(map[string]bool)
	var walk func(string) bool
	walk = func(id string) bool {
		for _, next := range edges[id] {
			if next == start {
				return true
			}
			if seen[next] {
				continue
			}
			seen[next] = true
			if walk(next) {
				return true
			}
		}
		return false
	}
	return walk(start)
}

// cyclePath returns one simple cycle that contains start, normalized so
// the smallest ID is first. It returns nil when start cannot reach itself.
func cyclePath(start string, edges map[string][]string) []string {
	type frame struct {
		id   string
		next int
	}
	stack := []frame{{id: start}}
	pos := map[string]int{start: 0}
	for len(stack) > 0 {
		i := len(stack) - 1
		pres := edges[stack[i].id]
		if stack[i].next >= len(pres) {
			delete(pos, stack[i].id)
			stack = stack[:i]
			continue
		}
		next := pres[stack[i].next]
		stack[i].next++
		if idx, onStack := pos[next]; onStack {
			cyc := make([]string, 0, len(stack)-idx)
			for _, fr := range stack[idx:] {
				cyc = append(cyc, fr.id)
			}
			if contains(cyc, start) {
				return normalizeCycle(cyc)
			}
			continue
		}
		stack = append(stack, frame{id: next})
		pos[next] = len(stack) - 1
	}
	return nil
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func normalizeCycle(path []string) []string {
	if len(path) == 0 {
		return nil
	}
	minI := 0
	for i := 1; i < len(path); i++ {
		if path[i] < path[minI] {
			minI = i
		}
	}
	out := make([]string, 0, len(path))
	out = append(out, path[minI:]...)
	out = append(out, path[:minI]...)
	return out
}
