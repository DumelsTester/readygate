package report

import (
	"errors"
	"io"

	"github.com/DumelsTester/readygate/eval"
)

// ErrNotImplemented is returned by report functionality left for a later change.
var ErrNotImplemented = errors.New("report: not implemented")

// WriteJSON does not write a report. It returns ErrNotImplemented.
//
// A later change can encode eval.Result with encoding/json, keeping checklist
// order and the same status and reason codes as Text. Scoring should stay
// as it is. The arguments are ignored, including a nil writer.
func WriteJSON(_ io.Writer, _ eval.Result) error {
	return ErrNotImplemented
}
