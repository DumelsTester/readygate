# readygate

readygate evaluates a readiness checklist against prerequisites and time-bounded evidence, then explains why each check passed, failed, or is blocked.

It is meant for repeatable release and service-handover questions: was the build recorded, is that record still fresh, and which later sign-off is waiting on an earlier failure. The answers come only from the checklist, the evidence you pass in, and a time you choose. The same inputs always produce the same result.

This is a library for explaining a checklist. It is not a certification, an approval system, or a claim that a service is safe to ship.

## Evaluation rules

A check **passes** when every prerequisite passed and every evidence requirement has enough valid items at the evaluation time.

A check **fails** when its own definition or evidence is the problem:

- it has no prerequisites and no evidence requirements
- a requirement has an empty kind, a negative count, or a repeated kind
- it is on a prerequisite cycle
- a prerequisite ID is not in the checklist
- an evidence requirement is missing, expired, not yet valid, or short of the required count

A check is **blocked** when its definition is usable and a prerequisite is failed or blocked. Its own evidence is still classified, so the report shows both the upstream hold and the local evidence. Blocked wins over a local evidence gap: the check is waiting, and the evidence lines say whether that evidence would have been enough.

`Ready` is true only when every check passed and the checklist has no structural problems. A duplicated prerequisite does not fail the check by itself, but it does keep `Ready` false.

Evidence belongs to one check ID and one kind. An item for another check, or of another kind, does not count.

An item is **valid** at time T when T is inside `[ObservedAt, ExpiresAt)`. T equal to `ObservedAt` is valid. T equal to `ExpiresAt` is **expired**. A zero `ExpiresAt` does not expire. An item observed after T is **not yet valid**, which is not the same as missing. Missing means no item of that kind was recorded for the check.

`MinCount` below 1, including the zero value, means one valid item. An empty kind, a negative count, or a repeated kind makes that check's requirements invalid and suppresses evidence scoring for the check. The check fails, and `Ready` stays false.

Invalid check IDs (an empty checklist, an empty ID, or a duplicate ID) make the checklist ambiguous. Evaluation reports those problems and does not score individual checks, because the IDs do not identify a single check.

Prerequisite cycles are reported one cycle per check. The path starts at the lexicographically smallest ID in the cycle that depth-first search finds first, following prerequisites in listed order. Downstream checks that depend on a cycle are blocked.

Pass the evaluation time explicitly. readygate does not read the wall clock. The stored time is UTC; comparisons use the instant, so the zone of the input does not change expiry.

## Packages

| Package | Role |
| --- | --- |
| `rules` | `Checklist`, `Check`, and `Requirement`. `Validate` reports structural problems and prerequisite cycles. |
| `evidence` | `Item` and an in-memory `Set`. `StateAt` classifies an item at a caller-supplied time. |
| `eval` | `Evaluate` scores a checklist against an `EvidenceView` and returns a `Result` with one explained `CheckResult` per check. `*evidence.Set` implements `EvidenceView`. |
| `report` | `Text` renders the result. `WriteJSON` is not implemented. |

`eval.Result.Problems` uses `rules.Problem`. Evidence requirements carry `evidence.Item` values through `EvidenceView`. The text report prints `eval.Reason` codes and messages.

## Example

```go
at := time.Date(2026, 11, 19, 15, 0, 0, 0, time.UTC)
list := rules.Checklist{
    ID:   "svc-handover",
    Name: "Service handover",
    Checks: []rules.Check{
        {ID: "build", Title: "Reproducible build", Evidence: []rules.Requirement{{Kind: "build-log"}}},
        {ID: "tests", Title: "Automated tests", Prerequisites: []string{"build"}, Evidence: []rules.Requirement{{Kind: "test-report"}}},
        {ID: "review", Title: "Operator review", Prerequisites: []string{"tests"}, Evidence: []rules.Requirement{{Kind: "sign-off"}}},
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
    return err
}
fmt.Print(report.Text(result))
```

```text
ID: svc-handover
Name: Service handover
Evaluated: 2026-11-19T15:00:00Z
Ready: no

Problems:
none

Check: build
Title: Reproducible build
Status: passed
- evidence_satisfied: kind "build-log": valid 1, need 1, evidence build-1 [ci]

Check: tests
Title: Automated tests
Status: failed
- prerequisites_satisfied: build
- evidence_expired: kind "test-report": valid 0, need 1, expired 1, evidence tests-old [ci]

Check: review
Title: Operator review
Status: blocked
- prerequisite_not_passed: tests is failed
- evidence_satisfied: kind "sign-off": valid 1, need 1, evidence review-1 [ops]
```

`go test` runs this as `ExampleEvaluate`. The build is still inside its valid interval. The test report expired the day before the handover. Operator review has a current sign-off, and it is blocked because the tests did not pass.

## Limitations

- No persistence, network, or process runner. Evidence is whatever the caller puts in a `Set`, or in another `EvidenceView`.
- Not a policy engine and not a safety or compliance certification.
- The text report is the only renderer. Names and notes are flattened to one line.
- `Set.Add` must not run at the same time as other `Set` methods. Concurrent evaluation is safe when the view is not being modified.
- Checklists are expected to be human-sized. Cycle detection searches simple prerequisite paths.
- One reported cycle per check, not every cycle through that check.
- Item IDs are exact, case-sensitive strings.

## Not implemented

`report.WriteJSON` returns `report.ErrNotImplemented` and writes nothing.

A later change can add a JSON report that encodes `eval.Result` in checklist order with the same status and reason codes as `Text`, using `encoding/json`. Scoring rules should stay as they are. No JSON shape has been started.

## Tests

```sh
gofmt -w .
go test ./...
go vet ./...
go test -race ./...
```

The module uses the standard library only.
