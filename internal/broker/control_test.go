package broker

import (
	"testing"

	"safeterm/internal/contract"
)

// TEST-STREAM-018
func TestWebButtonAndSSHInterruptMapToSemanticTerminate(t *testing.T) {
	web := (WebControlAdapter{}).TerminateButton("session", "alice")
	ssh := (SSHControlAdapter{}).Interrupt("session", "alice")
	for name, request := range map[string]contract.ControlRequest{"web": web, "ssh": ssh} {
		if err := request.Validate(); err != nil {
			t.Fatalf("%s request: %v", name, err)
		}
		if request.Action != contract.ControlTerminate || request.SessionID != "session" {
			t.Fatalf("%s request = %+v", name, request)
		}
	}
	if web.Owner.BrokerType != "web" || ssh.Owner.BrokerType != "ssh" {
		t.Fatalf("adapter provenance lost: web=%+v ssh=%+v", web.Owner, ssh.Owner)
	}
}
