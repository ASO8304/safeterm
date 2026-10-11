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
	if got := shell.completionCandidates(context.Background(), "execute tr", "tr"); len(got) != 1 || got[0] != "trace" {
		t.Fatalf("operations=%v", got)
	}
	if got := shell.completionCandidates(context.Background(), "execute trace po", "po"); len(got) != 1 || got[0] != "port=" {
		t.Fatalf("parameters=%v", got)
	}
	viewer, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "bob", Roles: []string{"viewer"}}, bytes.NewReader(nil), &bytes.Buffer{})
	if got := viewer.completionCandidates(context.Background(), "execute tr", "tr"); len(got) != 0 {
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

func TestPositionalMappedArgumentAndHelpExample(t *testing.T) {
	operation := config.OperationDefinition{
		Name: "log", Parameters: []config.ParameterDefinition{{Name: "source", Type: config.ParamTypeString, Required: true, ValueMap: map[string]string{"service_A": "/var/log/service-a.log"}}},
	}
	arguments, err := parseOperationArguments(operation, []string{"service_A"})
	if err != nil || arguments["source"] != "service_A" {
		t.Fatalf("arguments=%v err=%v", arguments, err)
	}
	if got := operationExample(operation); got != "execute log source=service_A" {
		t.Fatalf("example=%q", got)
	}
}

func TestConsoleIdentityUsesConfiguredRoles(t *testing.T) {
	data := []byte(`{"users":[{"os_user":"safeterm","user_id":"console-operator","roles":["netops","viewer"]}]}`)
	principal, err := ParseConsolePrincipal(data, "safeterm", map[string]bool{"viewer": true, "netops": true})
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != "console-operator" || principal.BrokerType != "console" || strings.Join(principal.Roles, ",") != "netops,viewer" {
		t.Fatalf("principal=%+v", principal)
	}
	if _, err := ParseConsolePrincipal(data, "other", map[string]bool{"viewer": true, "netops": true}); err == nil {
		t.Fatal("unregistered OS user accepted")
	}
	catalog, _ := NewCatalog(brokerTestConfig())
	shell, err := NewShellForBroker(&fakeGateway{}, catalog, principal, bytes.NewReader(nil), &bytes.Buffer{}, nil, "console")
	if err != nil {
		t.Fatal(err)
	}
	request, err := shell.request("system_info", map[string]string{})
	if err != nil || request.Metadata.BrokerType != "console" || request.Metadata.UserID != "console-operator" {
		t.Fatalf("request=%+v err=%v", request, err)
	}
}
