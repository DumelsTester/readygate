// Package report renders an evaluation result.
//
// Text is a deterministic plain-text report. WriteJSON is reserved for a
// later change and does not render anything.
package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DumelsTester/readygate/eval"
)

// Text returns the plain-text report for result.
func Text(result eval.Result) string {
	var b strings.Builder
	if err := WriteText(&b, result); err != nil {
		return ""
	}
	return b.String()
}

// WriteText writes a plain-text report. Newlines in names, titles, and
// messages are flattened to spaces so each record stays on one line.
// The evaluated time is formatted in UTC.
func WriteText(w io.Writer, result eval.Result) error {
	var b strings.Builder
	fmt.Fprintf(&b, "ID: %s\n", oneLine(result.ChecklistID))
	fmt.Fprintf(&b, "Name: %s\n", oneLine(result.Name))
	fmt.Fprintf(&b, "Evaluated: %s\n", result.EvaluatedAt.UTC().Format(time.RFC3339))
	if result.Ready {
		b.WriteString("Ready: yes\n")
	} else {
		b.WriteString("Ready: no\n")
	}
	b.WriteString("\nProblems:\n")
	if len(result.Problems) == 0 {
		b.WriteString("none\n")
	} else {
		for _, problem := range result.Problems {
			if problem.CheckID == "" {
				fmt.Fprintf(&b, "- [%s] %s\n", problem.Code, oneLine(problem.Message))
			} else {
				fmt.Fprintf(&b, "- [%s] %s: %s\n", problem.Code, oneLine(problem.CheckID), oneLine(problem.Message))
			}
		}
	}
	b.WriteString("\n")
	if len(result.Checks) == 0 {
		b.WriteString("Checks:\nnone\n")
	} else {
		for i, check := range result.Checks {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "Check: %s\n", oneLine(check.ID))
			fmt.Fprintf(&b, "Title: %s\n", oneLine(check.Title))
			fmt.Fprintf(&b, "Status: %s\n", check.Status)
			if len(check.Reasons) == 0 {
				b.WriteString("- (no reasons)\n")
				continue
			}
			for _, reason := range check.Reasons {
				fmt.Fprintf(&b, "- %s\n", oneLine(reason.String()))
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
