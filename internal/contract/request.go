// Package contract defines the standardized data structures exchanged
// between Brokers and the Agent Core (see docs/requirements/REQ-001 and
// docs/decisions/ADR-002).
//
// Every Broker (CLI, SSH, Web, ...) must translate its own input into an
// ExecutionRequest before handing it to Agent Core. Agent Core never
// accepts anything outside of this contract, and never trusts the claims
// inside Metadata (especially ClaimedRoles) without independent
// verification — that verification is added in a later milestone
// (Policy Engine, M5). For M1 this package only defines the shape of
// the contract and the minimal invariants it must always satisfy.
package contract

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Metadata carries information about how a request arrived and who
// claims to have sent it.
type Metadata struct {
	BrokerType   string    `json:"broker_type"` // e.g. "cli", "ssh", "web"
	UserID       string    `json:"user_id"`
	ClaimedRoles []string  `json:"claimed_roles"`
	ReceivedAt   time.Time `json:"received_at"`
}

// ExecutionRequest is the single standardized contract that every Broker
// must produce and that Agent Core consumes.
type ExecutionRequest struct {
	RequestID string            `json:"request_id"`
	Command   string            `json:"command"` // logical operation name, e.g. "trace" — never a raw shell command
	Arguments map[string]string `json:"arguments"`
	Metadata  Metadata          `json:"metadata"`
}

// NewExecutionRequest builds a valid ExecutionRequest. It:
//   - rejects an empty Command, since Command is the minimum information
//     Agent Core needs to route the request at all (REQ-001, AC-1)
//   - auto-generates a unique RequestID (REQ-001, AC-2)
//   - normalizes a nil Arguments map to an empty map, so downstream code
//     (Validation, Translation) never has to nil-check (REQ-001, AC-3)
//   - auto-populates Metadata.ReceivedAt when the caller didn't set it
//     (REQ-001, AC-4)
func NewExecutionRequest(command string, args map[string]string, meta Metadata) (*ExecutionRequest, error) {
	if command == "" {
		return nil, errors.New("contract: command must not be empty")
	}
	if args == nil {
		args = map[string]string{}
	}
	if meta.ReceivedAt.IsZero() {
		meta.ReceivedAt = time.Now().UTC()
	}

	return &ExecutionRequest{
		RequestID: generateRequestID(),
		Command:   command,
		Arguments: args,
		Metadata:  meta,
	}, nil
}

// generateRequestID returns a random 16-byte hex-encoded identifier.
// Using crypto/rand (rather than a UUID library) keeps M1 dependency-free;
// this can be swapped for a proper UUID library later without changing
// the public contract, since RequestID is just an opaque string.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely in practice; fall back rather than panic,
		// since a missing RequestID must never crash the Agent.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
