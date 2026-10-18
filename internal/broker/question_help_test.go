package broker

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestQuestionHelpListsDescriptionsWithoutExecution(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	for _, tc := range []struct{ input, contains string }{
		{"?", "Execute an authorized command"},
		{"execute ?", "System information"},
		{"execute system ?", "parameters: none"},
	} {
		var output bytes.Buffer
		client := &fakeGateway{}
		shell, _ := NewShell(client, catalog, Principal{UserID: "viewer", Roles: []string{"viewer"}}, strings.NewReader(""), &output)
		exit, err := shell.executeLine(context.Background(), tc.input, nil)
		if err != nil || exit || !strings.Contains(output.String(), tc.contains) {
			t.Fatalf("%s: exit=%t err=%v output=%q", tc.input, exit, err, output.String())
		}
		if len(client.requests) != 0 || len(client.terminated) != 0 {
			t.Fatal("help dispatched an operation")
		}
		if tc.input == "execute ?" && strings.Contains(output.String(), "Capture traffic") {
			t.Fatal("unauthorized operation leaked")
		}
	}
}

func TestQuestionHelpRejectsUnauthorizedOperation(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	var output bytes.Buffer
	shell, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "viewer", Roles: []string{"viewer"}}, strings.NewReader(""), &output)
	if _, err := shell.executeLine(context.Background(), "execute trace ?", nil); err == nil {
		t.Fatal("unauthorized help accepted")
	}
	if strings.Contains(output.String(), "Capture traffic") {
		t.Fatal("unauthorized description leaked")
	}
}

func TestQuestionHelpFromEditorReturnsToPrompt(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	var output bytes.Buffer
	client := &fakeGateway{}
	shell, _ := NewShell(client, catalog, Principal{UserID: "viewer", Roles: []string{"viewer"}}, strings.NewReader("execute ?\rwhoami\rexit\r"), &output)
	if err := shell.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "System information") || !strings.Contains(output.String(), "user: viewer") || !strings.Contains(output.String(), "bye") {
		t.Fatalf("help did not return to prompt: %q", output.String())
	}
	if len(client.requests) != 0 {
		t.Fatal("help executed a command")
	}
}
