package rules_test

import (
	"slices"
	"testing"

	"github.com/DumelsTester/readygate/rules"
)

func TestRequirementNeed(t *testing.T) {
	if got := (rules.Requirement{Kind: "log"}).Need(); got != 1 {
		t.Fatalf("zero MinCount need = %d, want 1", got)
	}
	if got := (rules.Requirement{Kind: "log", MinCount: -3}).Need(); got != 1 {
		t.Fatalf("negative MinCount need = %d, want 1", got)
	}
	if got := (rules.Requirement{Kind: "log", MinCount: 2}).Need(); got != 2 {
		t.Fatalf("need = %d, want 2", got)
	}
}

func TestFormatCycle(t *testing.T) {
	if got := rules.FormatCycle(nil); got != "" {
		t.Fatalf("empty cycle = %q", got)
	}
	if got := rules.FormatCycle([]string{"a"}); got != "a -> a" {
		t.Fatalf("self cycle = %q", got)
	}
	if got := rules.FormatCycle([]string{"a", "b", "c"}); got != "a -> b -> c -> a" {
		t.Fatalf("cycle = %q", got)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		list      rules.Checklist
		ambiguous bool
		codes     []rules.ProblemCode
		messages  map[string]string // check ID or "" -> one message contains
		cycles    map[string]string
	}{
		{
			name:      "empty",
			ambiguous: true,
			codes:     []rules.ProblemCode{rules.ProblemEmptyChecklist},
		},
		{
			name: "empty id",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "ok", Evidence: []rules.Requirement{{Kind: "log"}}},
				{Title: "missing id"},
			}},
			ambiguous: true,
			codes:     []rules.ProblemCode{rules.ProblemEmptyID},
		},
		{
			name: "duplicate id stops before cycle analysis",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "a", Prerequisites: []string{"b"}},
				{ID: "a", Prerequisites: []string{"a"}},
				{ID: "b", Prerequisites: []string{"a"}},
			}},
			ambiguous: true,
			codes:     []rules.ProblemCode{rules.ProblemDuplicateID},
		},
		{
			name: "valid chain",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "build", Evidence: []rules.Requirement{{Kind: "log"}}},
				{ID: "ship", Prerequisites: []string{"build"}},
			}},
		},
		{
			name:  "no criteria",
			list:  rules.Checklist{Checks: []rules.Check{{ID: "loose", Title: "Loose"}}},
			codes: []rules.ProblemCode{rules.ProblemNoCriteria},
		},
		{
			name: "invalid requirements",
			list: rules.Checklist{Checks: []rules.Check{{
				ID: "review",
				Evidence: []rules.Requirement{
					{Kind: ""},
					{Kind: "sign-off", MinCount: -1},
					{Kind: "sign-off"},
					{Kind: "sign-off", MinCount: 2},
				},
			}}},
			codes: []rules.ProblemCode{
				rules.ProblemInvalidRequirement,
				rules.ProblemInvalidRequirement,
				rules.ProblemInvalidRequirement,
			},
		},
		{
			name: "duplicate and unknown prerequisite",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "build", Evidence: []rules.Requirement{{Kind: "log"}}},
				{
					ID:            "ship",
					Prerequisites: []string{"build", "build", "missing"},
					Evidence:      []rules.Requirement{{Kind: "ticket"}},
				},
			}},
			codes: []rules.ProblemCode{
				rules.ProblemDuplicatePrerequisite,
				rules.ProblemUnknownPrerequisite,
			},
		},
		{
			name: "self cycle",
			list: rules.Checklist{Checks: []rules.Check{{
				ID:            "a",
				Prerequisites: []string{"a"},
			}}},
			codes:  []rules.ProblemCode{rules.ProblemCycle},
			cycles: map[string]string{"a": "a -> a"},
		},
		{
			name: "two node cycle is normalized",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "b", Prerequisites: []string{"a"}},
				{ID: "a", Prerequisites: []string{"b"}},
			}},
			codes: []rules.ProblemCode{rules.ProblemCycle, rules.ProblemCycle},
			cycles: map[string]string{
				"a": "a -> b -> a",
				"b": "a -> b -> a",
			},
		},
		{
			name: "side cycle does not hide the cycle that contains the start",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "a", Prerequisites: []string{"b"}},
				{ID: "b", Prerequisites: []string{"c"}},
				{ID: "c", Prerequisites: []string{"b", "a"}},
			}},
			codes: []rules.ProblemCode{rules.ProblemCycle, rules.ProblemCycle, rules.ProblemCycle},
			cycles: map[string]string{
				"a": "a -> b -> c -> a",
				"b": "b -> c -> b",
				"c": "b -> c -> b",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.list.Validate()
			if !sameValidation(got, rules.Validate(tt.list)) {
				t.Fatal("Checklist.Validate and Validate differ")
			}
			if got.Ambiguous != tt.ambiguous {
				t.Fatalf("ambiguous = %v, want %v", got.Ambiguous, tt.ambiguous)
			}
			var codes []rules.ProblemCode
			for _, problem := range got.Problems {
				codes = append(codes, problem.Code)
			}
			if !slices.Equal(codes, tt.codes) {
				t.Fatalf("codes = %v, want %v", codes, tt.codes)
			}
			if tt.ambiguous && got.Cycles != nil {
				t.Fatalf("ambiguous validation has cycles: %v", got.Cycles)
			}
			for id, want := range tt.cycles {
				if !got.InCycle(id) {
					t.Fatalf("check %s is not marked cyclic", id)
				}
				if got := rules.FormatCycle(got.Cycles[id]); got != want {
					t.Fatalf("cycle %s = %q, want %q", id, got, want)
				}
			}
			if len(tt.cycles) == 0 && len(got.Cycles) != 0 {
				t.Fatalf("unexpected cycles: %v", got.Cycles)
			}
		})
	}
}

func sameValidation(a, b rules.Validation) bool {
	if a.Ambiguous != b.Ambiguous || len(a.Problems) != len(b.Problems) || len(a.Cycles) != len(b.Cycles) {
		return false
	}
	for i := range a.Problems {
		if a.Problems[i] != b.Problems[i] {
			return false
		}
	}
	for id, path := range a.Cycles {
		if !slices.Equal(path, b.Cycles[id]) {
			return false
		}
	}
	return true
}

func TestProblemString(t *testing.T) {
	withCheck := (rules.Problem{
		Code: rules.ProblemUnknownPrerequisite, CheckID: "ship", Message: "prerequisite \"build\" does not exist",
	}).String()
	if withCheck != "unknown_prerequisite: ship: prerequisite \"build\" does not exist" {
		t.Fatal(withCheck)
	}
	bare := (rules.Problem{Code: rules.ProblemEmptyChecklist, Message: "checklist has no checks"}).String()
	if bare != "empty_checklist: checklist has no checks" {
		t.Fatal(bare)
	}
}
