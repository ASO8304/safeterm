package broker

import (
	"context"
	"safeterm/internal/terminalconfig"
	"strings"
	"testing"
)

func answerBytes(answer string) <-chan byte {
	ch := make(chan byte, len(answer))
	for _, b := range []byte(answer) {
		ch <- b
	}
	close(ch)
	return ch
}

func TestCustomPromptPreservesIdentityContextAndDirtyState(t *testing.T) {
	s, _, out := uxNetworkFixture(t)
	if err := s.SetPresentation(terminalconfig.Config{Name: "Acme", Prompt: "{brand}[{user}]{context}{dirty}$ "}); err != nil {
		t.Fatal(err)
	}
	if s.prompt() != "Acme[alice]$ " {
		t.Fatal(s.prompt())
	}
	editorCommands(t, s, "config network interface", "edit ip1", "set mtu 1400")
	if s.prompt() != "Acme[alice](network/interface/ip1)*$ " {
		t.Fatal(s.prompt())
	}
	s.networkEdit = nil
	s.input = strings.NewReader("exit\r")
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Acme restricted shell") {
		t.Fatal(out.String())
	}
}

func TestExitConfirmationKeepsDraftUnlessExplicitlyAccepted(t *testing.T) {
	for _, answer := range []string{"\r", "n\r", "no\r", "unexpected\r", "\x03", ""} {
		s, _, _ := uxNetworkFixture(t)
		editorCommands(t, s, "config network interface", "edit ip1", "set mtu 1400")
		before := s.prompt()
		exit, err := s.executeLine(context.Background(), "exit", answerBytes(answer))
		if exit || err != nil || s.prompt() != before || !s.editorDirty() {
			t.Fatalf("answer %q lost draft: %v %v", answer, exit, err)
		}
		exit, err = s.executeLine(context.Background(), "exit", answerBytes("yes\r"))
		if exit || err != nil || s.networkEdit.item != "" || s.editorDirty() {
			t.Fatal("confirmed exit failed", exit, err)
		}
	}
}

func TestExitCleanItemDoesNotDiscardOrConfirmOtherStagedItems(t *testing.T) {
	s, _, out := uxNetworkFixture(t)
	editorCommands(t, s, "config network interface", "edit ip1", "set mtu 1400", "next", "edit ip0")
	out.Reset()
	_, err := s.executeLine(context.Background(), "exit", nil)
	if err != nil || s.networkEdit.item != "" || !s.editorDirty() {
		t.Fatal("clean item exit affected another draft", err)
	}
	if out.Len() != 0 {
		t.Fatal("clean navigation asked a question", out)
	}
}
