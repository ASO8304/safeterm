package broker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"safeterm/internal/config"
)

func TestEditorSupportsCursorDeleteHistoryAndPrompt(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	input := make(chan byte, 128)
	for _, value := range []byte("hep\x1b[D" + "l" + "\x1b[C\x7f\r") {
		input <- value
	}
	output := &bytes.Buffer{}
	shell, err := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, bytes.NewReader(nil), output)
	if err != nil {
		t.Fatal(err)
	}
	line, err := shell.readLine(context.Background(), input, shell.prompt(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if line != "hel" {
		t.Fatalf("line=%q", line)
	}
	if !strings.Contains(output.String(), "safeterm::alice> ") {
		t.Fatalf("output=%q", output.String())
	}
	for _, value := range []byte("\x1b[A\r") {
		input <- value
	}
	line, err = shell.readLine(context.Background(), input, shell.prompt(), false, true)
	if err != nil || line != "hel" {
		t.Fatalf("history line=%q err=%v", line, err)
	}
}

func TestEditorCompletesAuthorizedOperationsAndParameters(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	shell, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, bytes.NewReader(nil), &bytes.Buffer{})
	if got := shell.completionCandidates(context.Background(), "execute cap", "cap"); len(got) != 1 || got[0] != "capture" {
		t.Fatalf("operations=%v", got)
	}
	if got := shell.completionCandidates(context.Background(), "execute capture po", "po"); len(got) != 1 || got[0] != "port=" {
		t.Fatalf("parameters=%v", got)
	}
	viewer, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "bob", Roles: []string{"viewer"}}, bytes.NewReader(nil), &bytes.Buffer{})
	if got := viewer.completionCandidates(context.Background(), "execute cap", "cap"); len(got) != 0 {
		t.Fatalf("unauthorized completion=%v", got)
	}
}

func TestParseOperationArgumentsAllowsAliasOverride(t *testing.T) {
	minimum, maximum := 1, 1000
	operation := config.OperationDefinition{Parameters: []config.ParameterDefinition{{Name: "count", Type: config.ParamTypeInt, Default: "100", Min: &minimum, Max: &maximum, Aliases: []string{"-c"}}}}
	arguments, err := parseOperationArguments(operation, []string{"-c", "5", "count=7"})
	if err != nil || arguments["count"] != "7" {
		t.Fatalf("arguments=%v err=%v, want final count=7", arguments, err)
	}
}

func TestTraceCompletionHasOnlyDynamicArguments(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	shell, err := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ before, prefix, want string }{
		{"execute capture ", "", "host=,port=,protocol="},
		{"execute capture po", "po", "port="},
		{"execute capture host=192.0.2.1 ", "", "port=,protocol="},
		{"execute capture port=999 ", "", "host=,protocol="},
		{"execute capture protocol=", "protocol=", "protocol=arp,protocol=icmp,protocol=ip,protocol=tcp,protocol=udp"},
		{"execute capture port=999 protocol=t", "protocol=t", "protocol=tcp"},
	} {
		if got := strings.Join(shell.completionCandidates(context.Background(), tc.before, tc.prefix), ","); got != tc.want {
			t.Errorf("completion(%q)=%q, want %q", tc.before, got, tc.want)
		}
	}
	for _, name := range []string{"trace_host", "trace_port", "trace_filtered"} {
		if _, ok := catalog.AuthorizedOperation(name, []string{"netops"}); ok {
			t.Errorf("obsolete operation %s remains", name)
		}
	}
}

func TestPositionalMappedArgumentAndHelpExample(t *testing.T) {
	operation := config.OperationDefinition{
		Name: "log", Parameters: []config.ParameterDefinition{{Name: "source", Type: config.ParamTypeString, Required: true, ValueMap: map[string]string{"service_A": "/var/log/service-a.log"}}},
	}
	arguments, err := parseOperationArguments(operation, []string{"service_A"})
	if err != nil || arguments["source"] != "service_A" {
		t.Fatalf("arguments=%v err=%v", arguments, err)
	}
	if got := operationExample(operation); got != "execute logs source=service_A" {
		t.Fatalf("example=%q", got)
	}
}

func TestConsoleRequestsCarryAuthenticatedSession(t *testing.T) {
	principal := Principal{UserID: "console-operator", Roles: []string{"netops", "viewer"}, BrokerType: "console", ConsoleSession: "opaque-token"}

	catalog, _ := NewCatalog(brokerTestConfig())
	shell, err := NewShellForBroker(&fakeGateway{}, catalog, principal, bytes.NewReader(nil), &bytes.Buffer{}, nil, "console")
	if err != nil {
		t.Fatal(err)
	}
	request, err := shell.request("system_info", map[string]string{})
	if err != nil || request.Metadata.BrokerType != "console" || request.Metadata.UserID != "console-operator" || request.Metadata.ConsoleSession != "opaque-token" {
		t.Fatalf("request=%+v err=%v", request, err)
	}
}

func TestPingCompletionOffersCanonicalParameters(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	shell, err := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ before, prefix, want string }{
		{"execute ping ", "", "count=,host="},
		{"execute ping host=192.168.222.10 ", "", "count="},
		{"execute ping host=192.168.222.10 co", "co", "count="},
		{"execute ping host=192.168.222.10 count=5 ", "", ""},
		{"execute ping count=5 ", "", "host="},
		{"execute ping count=", "count=", "count="},
		{"execute ping -", "-", "-c"},
	} {
		if got := strings.Join(shell.completionCandidates(context.Background(), tc.before, tc.prefix), ","); got != tc.want {
			t.Errorf("completion(%q)=%q, want %q", tc.before, got, tc.want)
		}
	}
}

func TestLogCompletionOffersOnlyMappedSources(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Operations = append(cfg.Operations, config.OperationDefinition{
		Name: "log", Description: "Test-only mapped log", RunnerProfile: config.ProfileLogReader,
		ExecutorKind: config.ExecutorProcess, ExecutableRef: "tail",
		ArgsTemplate: []string{"-n", "50", "-F", "--", "{source}"},
		Parameters: []config.ParameterDefinition{{Name: "source", Type: config.ParamTypeString, Required: true,
			ValueMap: map[string]string{"service_A": "/var/log/safeterm-readable/service_A.log", "system": "/var/log/syslog", "authentication": "/var/log/auth.log"}}},
		LongRunning: true,
	})
	for i := range cfg.Policies {
		if cfg.Policies[i].Role == "log-viewer" {
			cfg.Policies[i].Operations = []string{"log"}
		}
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	shell, err := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"log-viewer"}}, bytes.NewReader(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ before, prefix, want string }{
		{"execute logs ", "", "authentication,service_A,system"},
		{"execute logs s", "s", "service_A,system"},
		{"execute logs system ", "", ""},
		{"execute logs source=system ", "", ""},
		{"execute logs source=", "source=", "source=authentication,source=service_A,source=system"},
	} {
		if got := strings.Join(shell.completionCandidates(context.Background(), tc.before, tc.prefix), ","); got != tc.want {
			t.Errorf("completion(%q)=%q, want %q", tc.before, got, tc.want)
		}
	}
}

func TestRootShowIsRemovedWithoutChangingConfigShow(t *testing.T) {
	s, _, _ := editorFixture(t)
	client := &fakeGateway{}
	s.client = client
	for _, line := range []string{"show", "show capture", "show ?"} {
		if _, err := s.executeLine(context.Background(), line, nil); err == nil {
			t.Fatalf("root show accepted: %q", line)
		}
	}
	if got := s.completionCandidates(context.Background(), "sh", "sh"); len(got) != 0 {
		t.Fatalf("root show completed: %v", got)
	}
	if len(client.requests) != 0 {
		t.Fatal("root show dispatched execution")
	}
	editorCommands(t, s, "config options", "edit Server", "show", "cancel")
}
