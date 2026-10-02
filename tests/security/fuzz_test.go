package security_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/contract"
	"safeterm/internal/validation"
)

// TEST-SECURITY-005: REQ-014 AC-2 and AC-11.
func FuzzRequestValidationNeverPanicsOrAcceptsMetacharacters(f *testing.F) {
	for _, seed := range []string{"127.0.0.1", ";id", "$(id)", "a\nb", "<x>", strings.Repeat("x", 4096)} {
		f.Add(seed)
	}
	cfg, err := config.LoadConfig("../../configs")
	if err != nil {
		f.Fatal(err)
	}
	validator := validation.New(cfg)
	f.Fuzz(func(t *testing.T, value string) {
		request := &contract.ExecutionRequest{RequestID: "fuzz", Command: "trace", Arguments: map[string]string{"port": "53", "host": value}, Metadata: contract.Metadata{BrokerType: "ssh", UserID: "fuzzer", ClaimedRoles: []string{"netops"}, ReceivedAt: time.Now().UTC()}}
		err := validator.Validate(request)
		if strings.ContainsAny(value, ";&|`$(){}<>\n\r'\"") && err == nil {
			t.Fatalf("dangerous value accepted: %q", value)
		}
	})
}

// TEST-SECURITY-006: REQ-014 AC-2, AC-10, and AC-11.
func FuzzResourceRegistryValidationNeverPanics(f *testing.F) {
	for _, seed := range [][]byte{[]byte(`{}`), []byte(`{"version":"x","resources":[]}`), []byte(`{"version":"x","resources":[{"id":"../../etc"}]}`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var registry configmanager.Registry
		if json.Unmarshal(data, &registry) == nil {
			_ = configmanager.ValidateRegistry(&registry)
		}
	})
}

// TEST-SECURITY-007: REQ-014 AC-5 and AC-11.
func FuzzStreamFrameValidationNeverPanics(f *testing.F) {
	f.Add("stdout", "running", uint64(1), []byte("x"))
	f.Add("stdout", "running", uint64(0), make([]byte, contract.MaxStreamChunkBytes+1))
	f.Fuzz(func(t *testing.T, kind, state string, sequence uint64, data []byte) {
		frame := contract.StreamFrame{Version: contract.StreamProtocolVersion, Type: contract.StreamFrameType(kind), RequestID: "request", SessionID: "session", Sequence: sequence, Data: data, State: contract.SessionState(state), OccurredAt: time.Now().UTC()}
		_ = frame.Validate(contract.MaxStreamChunkBytes)
	})
}
