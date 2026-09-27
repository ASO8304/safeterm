package agent

import "fmt"

// Stage identifies which point in the request lifecycle produced an error.
// This is intentionally a typed string (not a plain error message) so that
// the future Audit milestone (M9) can log *which* stage rejected a request
// without parsing error text.
type Stage string

const (
	StageValidation  Stage = "validation"
	StagePolicy      Stage = "policy"
	StageTranslation Stage = "translation"
	StageExecution   Stage = "execution"
)

// StageError wraps an underlying error with the pipeline stage that
// produced it. Agent Core never lets a stage error pass silently —
// every rejection is traceable to exactly one stage.
type StageError struct {
	Stage Stage
	Err   error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

// Unwrap allows callers to use errors.Is / errors.As against the
// underlying cause, not just the stage wrapper.
func (e *StageError) Unwrap() error {
	return e.Err
}
