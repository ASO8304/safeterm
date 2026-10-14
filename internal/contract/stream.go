package contract

import (
	"fmt"
	"time"
)

const StreamProtocolVersion = 1
const MaxStreamChunkBytes = 16 << 10

type StreamFrameType string

const (
	FrameSessionStarted StreamFrameType = "session_started"
	FrameStdout         StreamFrameType = "stdout"
	FrameStderr         StreamFrameType = "stderr"
	FrameSessionExited  StreamFrameType = "session_exited"
)

type SessionState string

const (
	SessionStarting    SessionState = "starting"
	SessionRunning     SessionState = "running"
	SessionTerminating SessionState = "terminating"
	SessionExited      SessionState = "exited"
)

type ControlAction string

const ControlTerminate ControlAction = "terminate"

type ControlStatus string

const (
	ControlTerminating        ControlStatus = "terminating"
	ControlAlreadyTerminating ControlStatus = "already_terminating"
	ControlAlreadyExited      ControlStatus = "already_exited"
	ControlNotFound           ControlStatus = "not_found"
	ControlDenied             ControlStatus = "denied"
)

// SessionOwner is authenticated transport context when created by the M12 SSH
// Broker. It is never used as a process primitive.
type SessionOwner struct {
	ConsoleSession string `json:"console_session,omitempty"`
	BrokerType     string `json:"broker_type"`
	UserID         string `json:"user_id"`
}

func OwnerFromMetadata(metadata Metadata) SessionOwner {
	return SessionOwner{BrokerType: metadata.BrokerType, UserID: metadata.UserID, ConsoleSession: metadata.ConsoleSession}
}

type StreamFrame struct {
	Version    int                `json:"version"`
	Type       StreamFrameType    `json:"type"`
	RequestID  string             `json:"request_id"`
	SessionID  string             `json:"session_id"`
	Sequence   uint64             `json:"sequence"`
	Data       []byte             `json:"data,omitempty"`
	State      SessionState       `json:"state,omitempty"`
	Response   *ExecutionResponse `json:"response,omitempty"`
	OccurredAt time.Time          `json:"occurred_at"`
}

func (f StreamFrame) Validate(maxChunkBytes int) error {
	if f.Version != StreamProtocolVersion || f.RequestID == "" || f.SessionID == "" || f.OccurredAt.IsZero() {
		return fmt.Errorf("contract: invalid stream frame envelope")
	}
	switch f.Type {
	case FrameSessionStarted:
		if f.Sequence != 0 || len(f.Data) != 0 || f.State != SessionRunning || f.Response != nil {
			return fmt.Errorf("contract: invalid session_started frame")
		}
	case FrameStdout, FrameStderr:
		if f.Sequence == 0 || len(f.Data) == 0 || len(f.Data) > maxChunkBytes || f.State != SessionRunning || f.Response != nil {
			return fmt.Errorf("contract: invalid stream chunk")
		}
	case FrameSessionExited:
		if f.Sequence == 0 || len(f.Data) != 0 || f.State != SessionExited || f.Response == nil ||
			f.Response.RequestID != f.RequestID || f.Response.Result == "" ||
			f.Response.StartedAt.IsZero() || f.Response.EndedAt.IsZero() || f.Response.EndedAt.Before(f.Response.StartedAt) {
			return fmt.Errorf("contract: invalid session_exited frame")
		}
	default:
		return fmt.Errorf("contract: unsupported stream frame type %q", f.Type)
	}
	return nil
}

type ControlRequest struct {
	Version   int           `json:"version"`
	Action    ControlAction `json:"action"`
	SessionID string        `json:"session_id"`
	Owner     SessionOwner  `json:"owner"`
}

func (r ControlRequest) Validate() error {
	if r.Version != StreamProtocolVersion || r.Action != ControlTerminate || r.SessionID == "" || r.Owner.BrokerType == "" || r.Owner.UserID == "" {
		return fmt.Errorf("contract: invalid control request")
	}
	return nil
}

type ControlResult struct {
	Version   int           `json:"version"`
	SessionID string        `json:"session_id"`
	Status    ControlStatus `json:"status"`
}

func (r ControlResult) Validate() error {
	if r.Version != StreamProtocolVersion || r.SessionID == "" {
		return fmt.Errorf("contract: invalid control result")
	}
	switch r.Status {
	case ControlTerminating, ControlAlreadyTerminating, ControlAlreadyExited, ControlNotFound, ControlDenied:
		return nil
	default:
		return fmt.Errorf("contract: unsupported control status %q", r.Status)
	}
}
