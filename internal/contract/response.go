package contract

import "time"

// ExecutionResult represents how the operation concluded.
type ExecutionResult string

const (
	ResultSuccess        ExecutionResult = "success"
	ResultRejected       ExecutionResult = "rejected"        // failed validation or policy, never reached execution
	ResultExecutionError ExecutionResult = "execution_error" // ran, but the underlying command failed
)

// ExecutionResponse is what Agent Core returns for a given ExecutionRequest.
// It is intentionally symmetric to ExecutionRequest (same RequestID) so the
// pair can be correlated and logged together for audit purposes in the
// Audit milestone (M8).
type ExecutionResponse struct {
	RequestID string          `json:"request_id"`
	Result    ExecutionResult `json:"result"`
	Message   string          `json:"message,omitempty"`
	Stdout    string          `json:"stdout,omitempty"`
	Stderr    string          `json:"stderr,omitempty"`
	ExitCode  int             `json:"exit_code"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   time.Time       `json:"ended_at"`
}

// Duration is a convenience helper, mainly useful once Audit logging (M8)
// needs to record how long an operation took.
func (r ExecutionResponse) Duration() time.Duration {
	return r.EndedAt.Sub(r.StartedAt)
}
