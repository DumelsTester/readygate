package eval_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/DumelsTester/readygate/eval"
	"github.com/DumelsTester/readygate/evidence"
	"github.com/DumelsTester/readygate/rules"
)

func TestEvaluateRejectsBadInput(t *testing.T) {
	list := rules.Checklist{Checks: []rules.Check{{ID: "tests", Evidence: []rules.Requirement{{Kind: "test-report"}}}}}
	_, err := eval.Evaluate(list, evidence.New(), time.Time{})
	if !errors.Is(err, eval.ErrEvaluationTime) {
		t.Fatalf("zero time err = %v", err)
	}
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	_, err = eval.Evaluate(list, nil, at)
	if !errors.Is(err, eval.ErrNilEvidence) {
		t.Fatalf("nil evidence err = %v", err)
	}
}

func TestEvaluateAmbiguousChecklist(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	result, err := eval.Evaluate(rules.Checklist{}, evidence.New(), at)
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready || len(result.Checks) != 0 || len(result.Problems) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if !result.EvaluatedAt.Equal(at) {
		t.Fatalf("evaluated at = %v", result.EvaluatedAt)
	}
	dup := rules.Checklist{Checks: []rules.Check{
		{ID: "a", Prerequisites: []string{"a"}},
		{ID: "a"},
	}}
	result, err = eval.Evaluate(dup, evidence.New(), at)
	if err != nil || result.Ready || len(result.Checks) != 0 {
		t.Fatalf("duplicate result = %+v err=%v", result, err)
	}
	if result.Problems[0].Code != rules.ProblemDuplicateID {
		t.Fatalf("problems = %+v", result.Problems)
	}
}

func TestEvaluateScenarios(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	before := at.Add(-time.Hour)
	after := at.Add(time.Hour)
	pst := time.FixedZone("PST", -8*3600)
	atPST := at.In(pst)

	build := rules.Check{ID: "build", Title: "Build", Evidence: []rules.Requirement{{Kind: "build-log"}}}
	tests := rules.Check{ID: "tests", Title: "Tests", Prerequisites: []string{"build"}, Evidence: []rules.Requirement{{Kind: "test-report"}}}
	review := rules.Check{ID: "review", Title: "Review", Prerequisites: []string{"tests"}, Evidence: []rules.Requirement{{Kind: "sign-off"}}}

	type wantCheck struct {
		status eval.Status
		codes  []eval.Code
	}
	testsTable := []struct {
		name  string
		list  rules.Checklist
		items []evidence.Item
		at    time.Time
		ready bool
		want  map[string]wantCheck
	}{
		{
			name: "missing evidence fails and blocks dependents",
			list: rules.Checklist{ID: "rel", Checks: []rules.Check{build, tests}},
			at:   at,
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceMissing}},
				"tests": {eval.StatusBlocked, []eval.Code{eval.CodePrerequisiteNotPassed, eval.CodeEvidenceMissing}},
			},
		},
		{
			name: "expired evidence is not missing",
			list: rules.Checklist{Checks: []rules.Check{build}},
			at:   at,
			items: []evidence.Item{{
				ID: "old", CheckID: "build", Kind: "build-log", Source: "ci",
				ObservedAt: before, ExpiresAt: at,
			}},
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceExpired}},
			},
		},
		{
			name: "valid through the nanosecond before expiry",
			list: rules.Checklist{Checks: []rules.Check{build}},
			at:   at.Add(-time.Nanosecond),
			items: []evidence.Item{{
				ID: "log", CheckID: "build", Kind: "build-log",
				ObservedAt: before, ExpiresAt: at,
			}},
			ready: true,
			want: map[string]wantCheck{
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "not yet valid is distinct from expired",
			list: rules.Checklist{Checks: []rules.Check{build}},
			at:   at,
			items: []evidence.Item{{
				ID: "future", CheckID: "build", Kind: "build-log",
				ObservedAt: after, ExpiresAt: after.Add(time.Hour),
			}},
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceNotYetValid}},
			},
		},
		{
			name: "other check and other kind do not count",
			list: rules.Checklist{Checks: []rules.Check{build}},
			at:   at,
			items: []evidence.Item{
				{ID: "other-check", CheckID: "tests", Kind: "build-log", ObservedAt: before},
				{ID: "other-kind", CheckID: "build", Kind: "sign-off", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceMissing}},
			},
		},
		{
			name: "zone of the clock does not change the instant",
			list: rules.Checklist{Checks: []rules.Check{build}},
			at:   atPST,
			items: []evidence.Item{{
				ID: "log", CheckID: "build", Kind: "build-log",
				ObservedAt: before, ExpiresAt: at.Add(time.Nanosecond),
			}},
			ready: true,
			want: map[string]wantCheck{
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "need defaults to one and extra valid items still pass",
			list: rules.Checklist{Checks: []rules.Check{{
				ID: "build", Evidence: []rules.Requirement{{Kind: "build-log"}},
			}}},
			at: at,
			items: []evidence.Item{
				{ID: "b", CheckID: "build", Kind: "build-log", ObservedAt: before},
				{ID: "a", CheckID: "build", Kind: "build-log", ObservedAt: before},
			},
			ready: true,
			want: map[string]wantCheck{
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "one valid item is not enough when two are required and another is expired",
			list: rules.Checklist{Checks: []rules.Check{{
				ID: "build", Evidence: []rules.Requirement{{Kind: "build-log", MinCount: 2}},
			}}},
			at: at,
			items: []evidence.Item{
				{ID: "ok", CheckID: "build", Kind: "build-log", ObservedAt: before, ExpiresAt: after},
				{ID: "old", CheckID: "build", Kind: "build-log", ObservedAt: before, ExpiresAt: at},
			},
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceInsufficient}},
			},
		},
		{
			name: "mixed expired and not yet valid is insufficient",
			list: rules.Checklist{Checks: []rules.Check{build}},
			at:   at,
			items: []evidence.Item{
				{ID: "old", CheckID: "build", Kind: "build-log", ObservedAt: before, ExpiresAt: at},
				{ID: "future", CheckID: "build", Kind: "build-log", ObservedAt: after},
			},
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceInsufficient}},
			},
		},
		{
			name: "chain blocks on a blocked prerequisite",
			list: rules.Checklist{Checks: []rules.Check{build, tests, review}},
			at:   at,
			items: []evidence.Item{
				{ID: "tests-ok", CheckID: "tests", Kind: "test-report", ObservedAt: before},
				{ID: "review-ok", CheckID: "review", Kind: "sign-off", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"build":  {eval.StatusFailed, []eval.Code{eval.CodeEvidenceMissing}},
				"tests":  {eval.StatusBlocked, []eval.Code{eval.CodePrerequisiteNotPassed, eval.CodeEvidenceSatisfied}},
				"review": {eval.StatusBlocked, []eval.Code{eval.CodePrerequisiteNotPassed, eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "passed prerequisite with missing evidence fails the dependent",
			list: rules.Checklist{Checks: []rules.Check{build, tests}},
			at:   at,
			items: []evidence.Item{
				{ID: "log", CheckID: "build", Kind: "build-log", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
				"tests": {eval.StatusFailed, []eval.Code{eval.CodePrerequisitesSatisfied, eval.CodeEvidenceMissing}},
			},
		},
		{
			name: "unknown prerequisite fails rather than blocking",
			list: rules.Checklist{Checks: []rules.Check{{
				ID: "ship", Prerequisites: []string{"ghost"}, Evidence: []rules.Requirement{{Kind: "ticket"}},
			}}},
			at: at,
			items: []evidence.Item{
				{ID: "t", CheckID: "ship", Kind: "ticket", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"ship": {eval.StatusFailed, []eval.Code{eval.CodeUnknownPrerequisite, eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "cycle fails members and blocks a downstream check",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "a", Prerequisites: []string{"b"}},
				{ID: "b", Prerequisites: []string{"a"}},
				{ID: "ship", Prerequisites: []string{"a"}, Evidence: []rules.Requirement{{Kind: "ticket"}}},
			}},
			at: at,
			items: []evidence.Item{
				{ID: "t", CheckID: "ship", Kind: "ticket", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"a":    {eval.StatusFailed, []eval.Code{eval.CodeCycle}},
				"b":    {eval.StatusFailed, []eval.Code{eval.CodeCycle}},
				"ship": {eval.StatusBlocked, []eval.Code{eval.CodePrerequisiteNotPassed, eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "self prerequisite is a cycle",
			list: rules.Checklist{Checks: []rules.Check{{ID: "a", Prerequisites: []string{"a"}}}},
			at:   at,
			want: map[string]wantCheck{
				"a": {eval.StatusFailed, []eval.Code{eval.CodeCycle}},
			},
		},
		{
			name: "no criteria",
			list: rules.Checklist{Checks: []rules.Check{{ID: "loose"}}},
			at:   at,
			want: map[string]wantCheck{
				"loose": {eval.StatusFailed, []eval.Code{eval.CodeNoCriteria}},
			},
		},
		{
			name: "invalid requirement suppresses evidence scoring",
			list: rules.Checklist{Checks: []rules.Check{{
				ID: "build", Evidence: []rules.Requirement{{Kind: "build-log", MinCount: -1}},
			}}},
			at: at,
			items: []evidence.Item{
				{ID: "log", CheckID: "build", Kind: "build-log", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"build": {eval.StatusFailed, []eval.Code{eval.CodeInvalidRequirement}},
			},
		},
		{
			name: "duplicate prerequisite keeps the check passed and the run not ready",
			list: rules.Checklist{Checks: []rules.Check{
				build,
				{ID: "ship", Prerequisites: []string{"build", "build"}, Evidence: []rules.Requirement{{Kind: "ticket"}}},
			}},
			at: at,
			items: []evidence.Item{
				{ID: "log", CheckID: "build", Kind: "build-log", ObservedAt: before},
				{ID: "t", CheckID: "ship", Kind: "ticket", ObservedAt: before},
			},
			want: map[string]wantCheck{
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
				"ship":  {eval.StatusPassed, []eval.Code{eval.CodePrerequisitesSatisfied, eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "cycle still explains an external failed prerequisite",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "base", Evidence: []rules.Requirement{{Kind: "log"}}},
				{ID: "a", Prerequisites: []string{"b", "base"}},
				{ID: "b", Prerequisites: []string{"a"}},
			}},
			at: at,
			want: map[string]wantCheck{
				"base": {eval.StatusFailed, []eval.Code{eval.CodeEvidenceMissing}},
				"a":    {eval.StatusFailed, []eval.Code{eval.CodeCycle, eval.CodePrerequisiteNotPassed}},
				"b":    {eval.StatusFailed, []eval.Code{eval.CodeCycle}},
			},
		},
		{
			name: "results follow checklist order when dependencies are later",
			list: rules.Checklist{Checks: []rules.Check{
				{ID: "ship", Prerequisites: []string{"build"}, Evidence: []rules.Requirement{{Kind: "ticket"}}},
				build,
			}},
			at: at,
			items: []evidence.Item{
				{ID: "log", CheckID: "build", Kind: "build-log", ObservedAt: before},
				{ID: "t", CheckID: "ship", Kind: "ticket", ObservedAt: before},
			},
			ready: true,
			want: map[string]wantCheck{
				"ship":  {eval.StatusPassed, []eval.Code{eval.CodePrerequisitesSatisfied, eval.CodeEvidenceSatisfied}},
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
			},
		},
		{
			name: "prerequisite only check passes when upstream passes",
			list: rules.Checklist{Checks: []rules.Check{
				build,
				{ID: "gate", Prerequisites: []string{"build"}},
			}},
			at: at,
			items: []evidence.Item{
				{ID: "log", CheckID: "build", Kind: "build-log", ObservedAt: before},
			},
			ready: true,
			want: map[string]wantCheck{
				"build": {eval.StatusPassed, []eval.Code{eval.CodeEvidenceSatisfied}},
				"gate":  {eval.StatusPassed, []eval.Code{eval.CodePrerequisitesSatisfied}},
			},
		},
	}

	for _, tt := range testsTable {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set := evidence.New()
			for _, item := range tt.items {
				if err := set.Add(item); err != nil {
					t.Fatal(err)
				}
			}
			first, err := eval.Evaluate(tt.list, set, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			second, err := eval.Evaluate(tt.list, set, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("results differ:\n%#v\n%#v", first, second)
			}
			if first.Ready != tt.ready {
				t.Fatalf("ready = %v, want %v\n%s", first.Ready, tt.ready, explain(first))
			}
			if len(first.Checks) != len(tt.list.Checks) {
				t.Fatalf("got %d checks", len(first.Checks))
			}
			for i, check := range tt.list.Checks {
				if first.Checks[i].ID != check.ID {
					t.Fatalf("check order = %s, want %s", first.Checks[i].ID, check.ID)
				}
				got, ok := first.Find(check.ID)
				if !ok {
					t.Fatalf("missing %s", check.ID)
				}
				want := tt.want[check.ID]
				if got.Status != want.status {
					t.Fatalf("%s status = %s, want %s\n%s", check.ID, got.Status, want.status, got.Explain())
				}
				var codes []eval.Code
				for _, reason := range got.Reasons {
					codes = append(codes, reason.Code)
				}
				if !reflect.DeepEqual(codes, want.codes) {
					t.Fatalf("%s codes = %v, want %v\n%s", check.ID, codes, want.codes, got.Explain())
				}
			}
		})
	}
}

func TestEvaluateEvidenceDetails(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	list := rules.Checklist{Checks: []rules.Check{{
		ID:    "build",
		Title: "Build",
		Evidence: []rules.Requirement{{
			Kind:     "build-log",
			MinCount: 2,
		}},
	}}}
	set := evidence.New()
	items := []evidence.Item{
		{ID: "ok", CheckID: "build", Kind: "build-log", Source: "ci", Note: "line\n2", ObservedAt: at.Add(-time.Hour), ExpiresAt: at.Add(time.Hour)},
		{ID: "old", CheckID: "build", Kind: "build-log", Source: "ci", ObservedAt: at.Add(-2 * time.Hour), ExpiresAt: at},
		{ID: "future", CheckID: "build", Kind: "build-log", ObservedAt: at.Add(time.Hour)},
	}
	for _, item := range items {
		if err := set.Add(item); err != nil {
			t.Fatal(err)
		}
	}
	result, err := eval.Evaluate(list, set, at)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := result.Find("build")
	if !ok || len(got.Reasons) != 1 {
		t.Fatal(got.Explain())
	}
	want := eval.Reason{
		Code:        eval.CodeEvidenceInsufficient,
		Message:     `kind "build-log": valid 1, need 2, expired 1, not yet valid 1, valid evidence ok [ci] {line 2}, expired evidence old [ci], not yet valid evidence future`,
		Kind:        "build-log",
		Need:        2,
		Valid:       1,
		Expired:     1,
		NotYetValid: 1,
		EvidenceIDs: []string{"ok"},
	}
	if !reflect.DeepEqual(got.Reasons[0], want) {
		t.Fatalf("reason = %#v\nwant %#v", got.Reasons[0], want)
	}
}

func TestEvaluateSortsCustomView(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	view := stubView{items: []evidence.Item{
		{ID: "z", CheckID: "build", Kind: "build-log", ObservedAt: at.Add(-time.Hour)},
		{ID: "a", CheckID: "build", Kind: "build-log", ObservedAt: at.Add(-time.Hour)},
	}}
	list := rules.Checklist{Checks: []rules.Check{{ID: "build", Evidence: []rules.Requirement{{Kind: "build-log", MinCount: 2}}}}}
	result, err := eval.Evaluate(list, view, at)
	if err != nil {
		t.Fatal(err)
	}
	reason := result.Checks[0].Reasons[0]
	if reason.Code != eval.CodeEvidenceSatisfied {
		t.Fatal(reason)
	}
	wantIDs := []string{"a", "z"}
	if !reflect.DeepEqual(reason.EvidenceIDs, wantIDs) {
		t.Fatalf("ids = %v", reason.EvidenceIDs)
	}
	if reason.Message != `kind "build-log": valid 2, need 2, evidence a, z` {
		t.Fatalf("message = %s", reason.Message)
	}
}

func TestEvaluateConcurrentReaders(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	set := evidence.New()
	if err := set.Add(evidence.Item{
		ID: "log", CheckID: "build", Kind: "build-log", ObservedAt: at.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	list := rules.Checklist{Checks: []rules.Check{{ID: "build", Evidence: []rules.Requirement{{Kind: "build-log"}}}}}
	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := eval.Evaluate(list, set, at)
			if err != nil {
				errCh <- err
				return
			}
			if !result.Ready {
				errCh <- errors.New("expected ready")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestNilSetViewIsEmptyEvidence(t *testing.T) {
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	var set *evidence.Set
	list := rules.Checklist{Checks: []rules.Check{{ID: "build", Evidence: []rules.Requirement{{Kind: "build-log"}}}}}
	result, err := eval.Evaluate(list, set, at)
	if err != nil {
		t.Fatal(err)
	}
	if result.Checks[0].Status != eval.StatusFailed || result.Checks[0].Reasons[0].Code != eval.CodeEvidenceMissing {
		t.Fatalf("result = %+v", result.Checks[0])
	}
}

type stubView struct {
	items []evidence.Item
}

func (s stubView) Matching(checkID, kind string) []evidence.Item {
	var out []evidence.Item
	for _, item := range s.items {
		if item.CheckID == checkID && item.Kind == kind {
			out = append(out, item)
		}
	}
	return out
}

func explain(result eval.Result) string {
	out := ""
	for _, check := range result.Checks {
		out += check.ID + " " + string(check.Status) + "\n" + check.Explain() + "\n"
	}
	return out
}
