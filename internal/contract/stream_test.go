package contract

import (
	"testing"
	"time"
)

// TEST-STREAM-002
func TestStreamAndControlContractsRejectProcessPrimitives(t *testing.T) {
	frame := StreamFrame{
		Version: StreamProtocolVersion, Type: FrameStdout, RequestID: "request",
		SessionID: "session", Sequence: 1, Data: []byte("data"),
		State: SessionRunning, OccurredAt: time.Now().UTC(),
	}
	if err := frame.Validate(4); err != nil {
		t.Fatal(err)
	}
	frame.Data = []byte("oversized")
	if err := frame.Validate(4); err == nil {
		t.Fatal("oversized frame accepted")
	}
	control := ControlRequest{Version: StreamProtocolVersion, Action: ControlTerminate, SessionID: "opaque", Owner: SessionOwner{BrokerType: "web", UserID: "alice"}}
	if err := control.Validate(); err != nil {
		t.Fatal(err)
	}
	control.Action = "signal"
	if err := control.Validate(); err == nil {
		t.Fatal("raw signal-like control accepted")
	}
}

func TestExitedFrameRequiresCorrelatedCompleteResponse(t *testing.T) {
	now := time.Now().UTC()
	frame := StreamFrame{
		Version: StreamProtocolVersion, Type: FrameSessionExited, RequestID: "request",
		SessionID: "session", Sequence: 1, State: SessionExited, OccurredAt: now,
		Response: &ExecutionResponse{RequestID: "other", Result: ResultSuccess, StartedAt: now, EndedAt: now},
	}
	if err := frame.Validate(MaxStreamChunkBytes); err == nil {
		t.Fatal("mismatched final response was accepted")
	}
	frame.Response.RequestID = frame.RequestID
	if err := frame.Validate(MaxStreamChunkBytes); err != nil {
		t.Fatal(err)
	}
}
