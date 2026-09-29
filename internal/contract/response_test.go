package contract

import (
	"encoding/json"
	"testing"
	"time"
)

// TEST-CONTRACT-006
// Requirement: REQ-001 (AC-6)
// Scenario: Duration() must correctly compute the elapsed time between
// StartedAt and EndedAt, since this value will be written into the
// structured audit log in M9.
func TestExecutionResponse_Duration(t *testing.T) {
	start := time.Now()
	end := start.Add(2 * time.Second)
	resp := ExecutionResponse{StartedAt: start, EndedAt: end}

	if resp.Duration() != 2*time.Second {
		t.Fatalf("expected duration of 2s, got %v", resp.Duration())
	}
}

// TEST-CONTRACT-007
// Requirement: REQ-001 (AC-7)
func TestExecutionResponse_FailureStageSurvivesJSONRoundTrip(t *testing.T) {
	original := ExecutionResponse{RequestID: "request-1", Result: ResultRejected, FailureStage: "policy"}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ExecutionResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.FailureStage != "policy" || decoded.RequestID != original.RequestID || decoded.Result != original.Result {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
}
