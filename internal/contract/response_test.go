package contract

import (
	"testing"
	"time"
)

// TEST-CONTRACT-006
// Requirement: REQ-001 (AC-6)
// Scenario: Duration() must correctly compute the elapsed time between
// StartedAt and EndedAt, since this value will be written into the
// structured audit log in M8.
func TestExecutionResponse_Duration(t *testing.T) {
	start := time.Now()
	end := start.Add(2 * time.Second)
	resp := ExecutionResponse{StartedAt: start, EndedAt: end}

	if resp.Duration() != 2*time.Second {
		t.Fatalf("expected duration of 2s, got %v", resp.Duration())
	}
}
