package report_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/DumelsTester/readygate/eval"
	"github.com/DumelsTester/readygate/report"
)

func TestWriteJSONNotImplemented(t *testing.T) {
	result := eval.Result{
		ChecklistID: "rel",
		EvaluatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Ready:       true,
	}
	var buf bytes.Buffer
	err := report.WriteJSON(&buf, result)
	if !errors.Is(err, report.ErrNotImplemented) {
		t.Fatalf("err = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q", buf.String())
	}
	if err := report.WriteJSON(nil, result); !errors.Is(err, report.ErrNotImplemented) {
		t.Fatalf("nil writer err = %v", err)
	}
}
