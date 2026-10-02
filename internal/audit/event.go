// Package audit defines and persists security audit events for SafeTerm.
package audit

import (
	"context"
	"fmt"
	"sort"
	"time"

	"safeterm/internal/contract"
)

const SchemaVersion = 1

const (
	ComponentGateway       = "gateway"
	ComponentRunner        = "runner"
	ComponentConfigManager = "config-manager"
)

// Event is the stable, transport-neutral audit contract. It deliberately
// records argument names rather than values so credentials and other caller
// data do not become part of the audit trail.
type Event struct {
	SchemaVersion  int                      `json:"schema_version"`
	RequestID      string                   `json:"request_id"`
	SessionID      string                   `json:"session_id,omitempty"`
	CatalogVersion string                   `json:"catalog_version"`
	Component      string                   `json:"component"`
	Profile        string                   `json:"profile,omitempty"`
	Operation      string                   `json:"operation"`
	Outcome        string                   `json:"outcome"`
	BrokerType     string                   `json:"broker_type,omitempty"`
	UserID         string                   `json:"user_id,omitempty"`
	ClaimedRoles   []string                 `json:"claimed_roles,omitempty"`
	ArgumentNames  []string                 `json:"argument_names,omitempty"`
	Result         contract.ExecutionResult `json:"result,omitempty"`
	FailureStage   string                   `json:"failure_stage,omitempty"`
	ExitCode       *int                     `json:"exit_code,omitempty"`
	ReceivedAt     *time.Time               `json:"received_at,omitempty"`
	StartedAt      *time.Time               `json:"started_at,omitempty"`
	EndedAt        *time.Time               `json:"ended_at,omitempty"`
	DurationMillis *int64                   `json:"duration_ms,omitempty"`
	ResourceID     string                   `json:"resource_id,omitempty"`
	Field          string                   `json:"field,omitempty"`
	BaseVersion    string                   `json:"base_version,omitempty"`
	NewVersion     string                   `json:"new_version,omitempty"`
	ProposalID     string                   `json:"proposal_id,omitempty"`
	ApprovalUserID string                   `json:"approval_user_id,omitempty"`
	OccurredAt     time.Time                `json:"occurred_at"`
}

// Sink is the synchronous boundary used by Gateway and Runner. A failure is
// returned to the caller so an operation cannot start without its pre-execute
// audit record being durably accepted.
type Sink interface {
	Record(context.Context, Event) error
}

// NewEvent builds a sanitized event from a request and optional response.
func NewEvent(req *contract.ExecutionRequest, catalogVersion, component, profile, outcome string, response *contract.ExecutionResponse) Event {
	event := Event{
		SchemaVersion: SchemaVersion, CatalogVersion: catalogVersion,
		Component: component, Profile: profile, Outcome: outcome,
		OccurredAt: time.Now().UTC(),
	}
	if req != nil {
		event.RequestID = req.RequestID
		event.Operation = req.Command
		event.BrokerType = req.Metadata.BrokerType
		event.UserID = req.Metadata.UserID
		event.ClaimedRoles = append([]string(nil), req.Metadata.ClaimedRoles...)
		event.ReceivedAt = timePointer(req.Metadata.ReceivedAt)
		event.ArgumentNames = make([]string, 0, len(req.Arguments))
		for name := range req.Arguments {
			event.ArgumentNames = append(event.ArgumentNames, name)
		}
		sort.Strings(event.ArgumentNames)
	}
	if response != nil {
		event.Result = response.Result
		event.FailureStage = response.FailureStage
		if response.Result != contract.ResultRejected {
			exitCode := response.ExitCode
			event.ExitCode = &exitCode
		}
		event.StartedAt = timePointer(response.StartedAt)
		event.EndedAt = timePointer(response.EndedAt)
		if !response.StartedAt.IsZero() && !response.EndedAt.IsZero() {
			duration := response.Duration().Milliseconds()
			event.DurationMillis = &duration
		}
	}
	return event
}

// NewSessionEvent extends the sanitized lifecycle contract with an opaque
// streaming session identifier. Stream bytes are never accepted by this API.
func NewSessionEvent(req *contract.ExecutionRequest, catalogVersion, component, profile, sessionID, outcome string, response *contract.ExecutionResponse) Event {
	event := NewEvent(req, catalogVersion, component, profile, outcome, response)
	event.SessionID = sessionID
	return event
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copyValue := value.UTC()
	return &copyValue
}

func (e Event) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("audit: unsupported schema version %d", e.SchemaVersion)
	}
	if e.RequestID == "" || e.CatalogVersion == "" || e.Operation == "" || e.Outcome == "" || e.OccurredAt.IsZero() {
		return fmt.Errorf("audit: request ID, catalog version, operation, outcome, and timestamp are required")
	}
	if !isUTC(e.OccurredAt) || (e.ReceivedAt != nil && !isUTC(*e.ReceivedAt)) || (e.StartedAt != nil && !isUTC(*e.StartedAt)) || (e.EndedAt != nil && !isUTC(*e.EndedAt)) {
		return fmt.Errorf("audit: timestamps must use UTC")
	}
	if e.Component != ComponentGateway && e.Component != ComponentRunner && e.Component != ComponentConfigManager {
		return fmt.Errorf("audit: unsupported component %q", e.Component)
	}
	allowedOutcomes := map[string]map[string]bool{
		ComponentGateway: {
			"unknown-operation": true, "profile-unavailable": true, "routed": true,
			"runner-error": true, "returned": true,
		},
		ComponentRunner: {
			"profile-rejected": true, "accepted": true, "completed": true,
			"session-started": true, "stream-opened": true,
			"terminate-requested": true, "interrupt-delivered": true,
			"grace-expired": true, "forced-kill": true,
			"output-limit": true, "backpressure-overflow": true,
			"idle-timeout": true, "client-disconnected": true,
			"session-exited": true,
		},
		ComponentConfigManager: {
			"read": true, "validation-rejected": true, "preview-created": true,
			"apply-rejected": true, "apply-authorized": true, "applied": true, "activation-succeeded": true,
			"activation-failed": true, "rollback-authorized": true, "rollback-succeeded": true, "rollback-failed": true,
		},
	}
	if !allowedOutcomes[e.Component][e.Outcome] {
		return fmt.Errorf("audit: unsupported %s outcome %q", e.Component, e.Outcome)
	}
	if e.Component == ComponentConfigManager {
		if e.ResourceID == "" || e.UserID == "" || e.Profile == "" {
			return fmt.Errorf("audit: config-manager event requires resource, user, and profile")
		}
		if e.Result != "" || e.ExitCode != nil || e.StartedAt != nil || e.EndedAt != nil || e.DurationMillis != nil || e.FailureStage != "" {
			return fmt.Errorf("audit: config-manager event cannot contain execution response data")
		}
		return nil
	}
	sessionOutcome := map[string]bool{
		"session-started": true, "stream-opened": true, "terminate-requested": true,
		"interrupt-delivered": true, "grace-expired": true, "forced-kill": true,
		"output-limit": true, "backpressure-overflow": true, "idle-timeout": true,
		"client-disconnected": true, "session-exited": true,
	}
	if sessionOutcome[e.Outcome] && e.SessionID == "" {
		return fmt.Errorf("audit: streaming outcome %q requires a session ID", e.Outcome)
	}
	if e.Result != "" && e.Result != contract.ResultSuccess && e.Result != contract.ResultRejected && e.Result != contract.ResultExecutionError {
		return fmt.Errorf("audit: unsupported result %q", e.Result)
	}
	completion := e.Outcome == "returned" || e.Outcome == "completed" || e.Outcome == "profile-rejected" || e.Outcome == "session-exited"
	if completion {
		if e.Result == "" || e.StartedAt == nil || e.EndedAt == nil || e.DurationMillis == nil {
			return fmt.Errorf("audit: outcome %q requires complete response data", e.Outcome)
		}
		if (e.Result == contract.ResultRejected && e.ExitCode != nil) || (e.Result != contract.ResultRejected && e.ExitCode == nil) {
			return fmt.Errorf("audit: exit code presence does not match result %q", e.Result)
		}
		if e.EndedAt.Before(*e.StartedAt) || *e.DurationMillis != e.EndedAt.Sub(*e.StartedAt).Milliseconds() {
			return fmt.Errorf("audit: inconsistent response timing")
		}
		if e.Result == contract.ResultSuccess && e.FailureStage != "" {
			return fmt.Errorf("audit: successful result cannot have a failure stage")
		}
		if e.Result != contract.ResultSuccess && !validFailureStage(e.FailureStage) {
			return fmt.Errorf("audit: failed result requires a supported failure stage")
		}
	} else if e.Result != "" || e.ExitCode != nil || e.StartedAt != nil || e.EndedAt != nil || e.DurationMillis != nil || e.FailureStage != "" {
		return fmt.Errorf("audit: outcome %q cannot contain response data", e.Outcome)
	}
	return nil
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}

func validFailureStage(stage string) bool {
	switch stage {
	case "profile", "validation", "policy", "translation", "execution":
		return true
	default:
		return false
	}
}
