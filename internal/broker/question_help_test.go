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
		{"execute system_info host ?", "parameters: none"},
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
	if _, err := shell.executeLine(context.Background(), "execute capture ?", nil); err == nil {
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
	shell, _ := NewShell(client, catalog, Principal{UserID: "viewer", Roles: []string{"viewer"}}, strings.NewReader("execute ?\r\x15whoami\rexit\r"), &output)
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

func TestQuestionHelpRetainsEditablePrefixAndNeverDispatches(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"execute ?\rsystem_info host\r", "execute system_info host"},
		{"execute ?\r?\rsystem_info host\r\n", "execute system_info host"},
		{"execute system_info ?\rhost\r", "execute system_info host"},
		{"execute ?\r\n\x15whoami\r\n", "whoami"},
		{"?\rwhoami\r", "whoami"},
		{"execute forbidden ?\r\x15whoami\r", "whoami"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			catalog, _ := NewCatalog(brokerTestConfig())
			out := &bytes.Buffer{}
			client := &fakeGateway{}
			s, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), out)
			input := make(chan byte, len(tc.input))
			for _, b := range []byte(tc.input) {
				input <- b
			}
			close(input)
			line, err := s.readLine(context.Background(), input, s.prompt(), false, true)
			if err != nil || line != tc.want {
				t.Fatalf("line=%q err=%v output=%s", line, err, out)
			}
			if len(client.requests) != 0 {
				t.Fatal("help dispatched an operation")
			}
			for _, h := range s.history {
				if strings.Contains(h, "?") {
					t.Fatal("help polluted history", h)
				}
			}
		})
	}
}

func TestQuestionHelpRetainsFieldPrefixWithoutStaging(t *testing.T) {
	s, _, out := editorFixture(t)
	editorCommands(t, s, "config options", "edit Server")
	input := make(chan byte, 64)
	for _, b := range []byte("set Port ?\r9090\r") {
		input <- b
	}
	close(input)
	line, err := s.readLine(context.Background(), input, s.prompt(), false, true)
	if err != nil || line != "set Port 9090" || len(s.editing.changes) != 0 {
		t.Fatalf("help staged a change or lost input: %q %v", line, err)
	}
	if !strings.Contains(out.String(), s.prompt()+"set Port ") {
		t.Fatal("prefix not redrawn")
	}
	editorCommands(t, s, line)
	if len(s.editing.changes) != 1 {
		t.Fatal("continued field input did not stage")
	}
}

func TestLoginShowsCommandMenuForSSH(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	out := &bytes.Buffer{}
	s, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader("exit\r"), out)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	menu, _, _ := strings.Cut(out.String(), s.prompt())
	for _, want := range []string{"?                        Show available commands", "execute                  Execute an authorized command", "config                   Select a configuration file"} {
		if !strings.Contains(menu, want) {
			t.Fatalf("login missing %q: %s", want, menu)
		}
	}
}
