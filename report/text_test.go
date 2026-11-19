package report_test

import (
	"strings"
	"testing"
	"time"

	"github.com/DumelsTester/readygate/eval"
	"github.com/DumelsTester/readygate/report"
	"github.com/DumelsTester/readygate/rules"
)

func TestTextFormatsProblemsAndFlattensNewlines(t *testing.T) {
	result := eval.Result{
		ChecklistID: "rel\n1",
		Name:        "Hand\rover",
		EvaluatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("PST", -8*3600)),
		Problems: []rules.Problem{
			{Code: rules.ProblemEmptyChecklist, Message: "checklist has no checks"},
			{Code: rules.ProblemCycle, CheckID: "a\nb", Message: "prerequisite cycle a -> b -> a"},
		},
		Checks: []eval.CheckResult{{
			ID:     "ship",
			Title:  "Ship\nnow",
			Status: eval.StatusFailed,
			Reasons: []eval.Reason{{
				Code:    eval.CodeEvidenceMissing,
				Message: "kind \"ticket\": valid 0, need 1",
			}},
		}},
	}
	got := report.Text(result)
	want := strings.Join([]string{
		"ID: rel 1",
		"Name: Hand over",
		"Evaluated: 2026-01-02T11:04:05Z",
		"Ready: no",
		"",
		"Problems:",
		"- [empty_checklist] checklist has no checks",
		"- [cycle] a b: prerequisite cycle a -> b -> a",
		"",
		"Check: ship",
		"Title: Ship now",
		"Status: failed",
		"- evidence_missing: kind \"ticket\": valid 0, need 1",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("report =\n%s\nwant =\n%s", got, want)
	}
}

func TestTextEmptyChecks(t *testing.T) {
	got := report.Text(eval.Result{
		EvaluatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	})
	if !strings.Contains(got, "Checks:\nnone\n") {
		t.Fatalf("report =\n%s", got)
	}
	if !strings.Contains(got, "Ready: no\n") {
		t.Fatalf("report =\n%s", got)
	}
}
