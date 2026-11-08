package broker

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"safeterm/internal/contract"
	"safeterm/internal/terminalconfig"
)

func TestResultSummaryCanBeEnabledWithoutChangingCommandOutput(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var out bytes.Buffer
		s := &Shell{output: &out}
		if err := s.SetPresentation(terminalconfig.Config{ShowResult: enabled}); err != nil {
			t.Fatal(err)
		}
		s.writeResponse(&contract.ExecutionResponse{Result: contract.ResultSuccess, Stdout: "command output\n", Stderr: "command warning\n", Message: "debug detail", ExitCode: 0})
		if !strings.HasPrefix(out.String(), "command output\r\ncommand warning\r\n") {
			t.Fatal("lost command output", out.String())
		}
		if enabled {
			if !strings.Contains(out.String(), "result: success exit=0 message=debug detail") {
				t.Fatal(out.String())
			}
		} else if out.String() != "command output\r\ncommand warning\r\n" {
			t.Fatal("quiet output contains metadata", out.String())
		}
	}
}

func TestQuietResultsKeepFailuresAndRejectionsVisible(t *testing.T) {
	for _, tc := range []struct {
		response *contract.ExecutionResponse
		want     string
	}{
		{&contract.ExecutionResponse{Result: contract.ResultExecutionError, ExitCode: 1, Stderr: "Connection refused.\n", Message: "execution: /proc/self/fd/7 exit status 1"}, "Connection refused.\r\nError: Command failed.\r\n"},
		{&contract.ExecutionResponse{Result: contract.ResultRejected, Message: "Permission denied for this operation."}, "Error: Permission denied for this operation.\r\n"},
		{&contract.ExecutionResponse{Result: contract.ResultRejected}, "Error: Command rejected.\r\n"},
		{&contract.ExecutionResponse{Result: contract.ResultSuccess, ExitCode: 1}, "Error: Command failed.\r\n"},
		{nil, "Error: Invalid command response.\r\n"},
	} {
		var out bytes.Buffer
		s := &Shell{output: &out}
		s.writeResponse(tc.response)
		if out.String() != tc.want {
			t.Fatalf("want %q, got %q", tc.want, out.String())
		}
	}
}

func TestStreamingResultSummaryFollowsPresentationSetting(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		catalog, _ := NewCatalog(brokerTestConfig())
		client := &fakeGateway{}
		var out bytes.Buffer
		s, err := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, strings.NewReader(""), &out)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetPresentation(terminalconfig.Config{ShowResult: enabled}); err != nil {
			t.Fatal(err)
		}
		input := make(chan byte, 1)
		input <- 0x03
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err = s.executeLine(ctx, "execute capture port=53", input)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "result: execution_error exit=130") != enabled {
			t.Fatal(enabled, out.String())
		}
		if !strings.Contains(out.String(), "packet") {
			t.Fatal("stream output lost", out.String())
		}
	}
}
