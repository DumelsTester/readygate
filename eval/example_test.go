package eval_test

import (
	"fmt"
	"time"

	"github.com/DumelsTester/readygate/eval"
	"github.com/DumelsTester/readygate/evidence"
	"github.com/DumelsTester/readygate/report"
	"github.com/DumelsTester/readygate/rules"
)

func ExampleEvaluate() {
	at := time.Date(2026, 11, 19, 15, 0, 0, 0, time.UTC)
	list := rules.Checklist{
		ID:   "svc-handover",
		Name: "Service handover",
		Checks: []rules.Check{
			{
				ID:       "build",
				Title:    "Reproducible build",
				Evidence: []rules.Requirement{{Kind: "build-log"}},
			},
			{
				ID:            "tests",
				Title:         "Automated tests",
				Prerequisites: []string{"build"},
				Evidence:      []rules.Requirement{{Kind: "test-report"}},
			},
			{
				ID:            "review",
				Title:         "Operator review",
				Prerequisites: []string{"tests"},
				Evidence:      []rules.Requirement{{Kind: "sign-off"}},
			},
		},
	}

	recorded := evidence.New()
	_ = recorded.Add(evidence.Item{
		ID: "build-1", CheckID: "build", Kind: "build-log", Source: "ci",
		ObservedAt: time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 12, 19, 15, 0, 0, 0, time.UTC),
	})
	_ = recorded.Add(evidence.Item{
		ID: "tests-old", CheckID: "tests", Kind: "test-report", Source: "ci",
		ObservedAt: time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC),
	})
	_ = recorded.Add(evidence.Item{
		ID: "review-1", CheckID: "review", Kind: "sign-off", Source: "ops",
		ObservedAt: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
	})

	result, err := eval.Evaluate(list, recorded, at)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(report.Text(result))
	// Output:
	// ID: svc-handover
	// Name: Service handover
	// Evaluated: 2026-11-19T15:00:00Z
	// Ready: no
	//
	// Problems:
	// none
	//
	// Check: build
	// Title: Reproducible build
	// Status: passed
	// - evidence_satisfied: kind "build-log": valid 1, need 1, evidence build-1 [ci]
	//
	// Check: tests
	// Title: Automated tests
	// Status: failed
	// - prerequisites_satisfied: build
	// - evidence_expired: kind "test-report": valid 0, need 1, expired 1, evidence tests-old [ci]
	//
	// Check: review
	// Title: Operator review
	// Status: blocked
	// - prerequisite_not_passed: tests is failed
	// - evidence_satisfied: kind "sign-off": valid 1, need 1, evidence review-1 [ops]
}

func ExampleEvaluate_ready() {
	at := time.Date(2026, 11, 19, 15, 0, 0, 0, time.UTC)
	list := rules.Checklist{
		ID:   "patch",
		Name: "Patch release",
		Checks: []rules.Check{{
			ID:       "tests",
			Title:    "Automated tests",
			Evidence: []rules.Requirement{{Kind: "test-report"}},
		}},
	}
	recorded := evidence.New()
	_ = recorded.Add(evidence.Item{
		ID: "run-14", CheckID: "tests", Kind: "test-report", Source: "ci",
		ObservedAt: at.Add(-time.Hour),
	})
	result, err := eval.Evaluate(list, recorded, at)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(report.Text(result))
	// Output:
	// ID: patch
	// Name: Patch release
	// Evaluated: 2026-11-19T15:00:00Z
	// Ready: yes
	//
	// Problems:
	// none
	//
	// Check: tests
	// Title: Automated tests
	// Status: passed
	// - evidence_satisfied: kind "test-report": valid 1, need 1, evidence run-14 [ci]
}
