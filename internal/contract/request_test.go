package contract

import (
	"encoding/json"
	"testing"
)

// TEST-CONTRACT-001
// Requirement: REQ-001 (AC-1)
// Scenario: NewExecutionRequest must reject an empty command name,
// since Agent Core cannot route a request it cannot identify.
func TestNewExecutionRequest_RejectsEmptyCommand(t *testing.T) {
	_, err := NewExecutionRequest("", nil, Metadata{})
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

// TEST-CONTRACT-002
// Requirement: REQ-001 (AC-2)
// Scenario: Two calls with the same command must produce different,
// non-empty RequestIDs (uniqueness is required for audit correlation later).
func TestNewExecutionRequest_GeneratesUniqueRequestID(t *testing.T) {
	req1, err := NewExecutionRequest("system_info", nil, Metadata{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req2, err := NewExecutionRequest("system_info", nil, Metadata{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req1.RequestID == "" {
		t.Fatal("expected non-empty RequestID")
	}
	if req1.RequestID == req2.RequestID {
		t.Fatal("expected two calls to produce different RequestIDs")
	}
}

// TEST-CONTRACT-003
// Requirement: REQ-001 (AC-3)
// Scenario: A nil Arguments map must be normalized to an empty, non-nil
// map, so Validation/Translation (future milestones) never have to
// nil-check before reading from it.
func TestNewExecutionRequest_NormalizesNilArguments(t *testing.T) {
	req, err := NewExecutionRequest("system_info", nil, Metadata{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Arguments == nil {
		t.Fatal("expected Arguments to be initialized, got nil")
	}
}

// TEST-CONTRACT-004
// Requirement: REQ-001 (AC-4)
// Scenario: Metadata.ReceivedAt must be auto-populated when the caller
// did not set it, so every request has a reliable timestamp for audit.
func TestNewExecutionRequest_AutoPopulatesReceivedAt(t *testing.T) {
	req, err := NewExecutionRequest("system_info", nil, Metadata{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Metadata.ReceivedAt.IsZero() {
		t.Fatal("expected ReceivedAt to be auto-populated")
	}
}

// TEST-CONTRACT-005
// Requirement: REQ-001 (AC-5)
// Scenario: ExecutionRequest must survive a JSON round-trip unchanged.
// This matters because Broker and Agent may end up as separate processes
// communicating over a serialized transport (HTTP, Unix socket, gRPC),
// even though in M1 they are just Go structs in the same binary.
func TestExecutionRequest_JSONRoundTrip(t *testing.T) {
	original, err := NewExecutionRequest("system_info", map[string]string{"foo": "bar"}, Metadata{
		BrokerType:   "cli",
		UserID:       "u123",
		ClaimedRoles: []string{"operator"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ExecutionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.RequestID != original.RequestID {
		t.Fatalf("RequestID mismatch after round-trip: got %s want %s", decoded.RequestID, original.RequestID)
	}
	if decoded.Command != original.Command {
		t.Fatal("Command mismatch after round-trip")
	}
	if decoded.Arguments["foo"] != "bar" {
		t.Fatal("Arguments mismatch after round-trip")
	}
	if len(decoded.Metadata.ClaimedRoles) != 1 || decoded.Metadata.ClaimedRoles[0] != "operator" {
		t.Fatal("Metadata.ClaimedRoles mismatch after round-trip")
	}
}
