package broker

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"safeterm/internal/contract"
	"safeterm/internal/terminalconfig"
)

var themeSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestTerminalThemeFallbacks(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	s := &Shell{principal: Principal{UserID: "admin"}, configMenu: []string{"network", "interface"}}
	plain := s.prompt()
	for _, terminal := range []string{"", "dumb", "linux", "xterm-256color"} {
		s.SetTerminal(terminal)
		got := s.prompt()
		if themeSGR.ReplaceAllString(got, "") != plain {
			t.Fatalf("%s: color changed prompt text: %q", terminal, got)
		}
		if strings.Contains(got, "\x1b[") != (terminal != "" && terminal != "dumb") {
			t.Fatalf("%s: unexpected color capability: %q", terminal, got)
		}
	}
	t.Setenv("NO_COLOR", "1")
	s.SetTerminal("xterm-256color")
	if s.prompt() != plain {
		t.Fatal("NO_COLOR must disable prompt styling")
	}
}

func TestThemePreservesTablesAndSanitizesText(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var out bytes.Buffer
	s := &Shell{output: &out}
	s.SetTerminal("linux")
	s.helpRow("edit", "Select interface")
	s.writeTable([]byte("NAME  STATE\nip0   up\n"))
	want := "edit                     Select interface\r\nNAME  STATE\r\nip0   up\r\n"
	if got := themeSGR.ReplaceAllString(out.String(), ""); got != want {
		t.Fatalf("layout changed: %q", got)
	}
	out.Reset()
	s.writeStyled(styleError, "Error: bad\x1b[2J\x07\n")
	if got := themeSGR.ReplaceAllString(out.String(), ""); got != "Error: bad�[2J�\r\n" {
		t.Fatalf("untrusted control sequence survived: %q", got)
	}
	out.Reset()
	s.writeTerminalData([]byte("\x1b[31muntrusted"))
	if strings.Contains(out.String(), "\x1b") {
		t.Fatal("operation output must not introduce ANSI styling")
	}
}

func TestColoredPromptRedrawPreservesInputAndCursor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var out bytes.Buffer
	s := &Shell{output: &out, principal: Principal{UserID: "admin"}, configMenu: []string{"network", "interface"}}
	s.SetTerminal("xterm-256color")
	input := make(chan byte, 32)
	for _, b := range []byte("shw\x1b[Do\r") {
		input <- b
	}
	line, err := s.readLine(context.Background(), input, s.prompt(), false, true)
	if err != nil || line != "show" {
		t.Fatalf("line=%q, err=%v", line, err)
	}
	if !strings.Contains(out.String(), s.prompt()+"show\x1b[K\x1b[1D") {
		t.Fatalf("colored prompt broke cursor redraw: %q", out.String())
	}
	if !strings.HasSuffix(s.prompt(), "\x1b[0m ") {
		t.Fatal("prompt must reset styling before typed input")
	}
}

func TestThemeKeepsNetworkTableLayoutAndDistinguishesStatuses(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "config network interface", "edit ip1", "set dhcp4 enable", "append addresses 10.0.0.5/24", "next")
	out.Reset()
	editorCommands(t, s, "show")
	plain := out.String()
	out.Reset()
	s.SetTerminal("xterm-256color")
	editorCommands(t, s, "show")
	colored := out.String()
	if themeSGR.ReplaceAllString(colored, "") != plain {
		t.Fatalf("table columns changed:\n%s", colored)
	}
	for _, want := range []string{"\x1b[32mup", "\x1b[33mdown", "\x1b[33mRead-only", "\x1b[32mAllowed", "\x1b[33m *\x1b[0m"} {
		if !strings.Contains(colored, want) {
			t.Fatalf("missing %q in %q", want, colored)
		}
	}
	if len(api.requests) != 1 {
		t.Fatal("coloring the table called the manager")
	}
}

func TestThemeKeepsHelpInputAndSecretEntryBehavior(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	s, _, out := editorFixture(t)
	s.SetTerminal("xterm-256color")
	input := make(chan byte, 128)
	for _, b := range []byte("execute ?\r\x15whoami\r") {
		input <- b
	}
	line, err := s.readLine(context.Background(), input, s.prompt(), false, true)
	if err != nil || line != "whoami" || !strings.Contains(out.String(), s.prompt()+"execute ") {
		t.Fatal(line, err, out)
	}
	out.Reset()
	for _, b := range []byte("secret-sentinel\r") {
		input <- b
	}
	line, err = s.readLine(context.Background(), input, s.styled(styleCommand, "Password: "), true, false)
	if err != nil || line != "secret-sentinel" || strings.Contains(out.String(), "secret-sentinel") {
		t.Fatal("secret input was echoed", err)
	}
}

func TestThemeTrustBoundaryIncludesPlainAndResponseMessages(t *testing.T) {
	for _, color := range []bool{false, true} {
		var out bytes.Buffer
		s := &Shell{color: color, output: &out, presentation: terminalconfig.Config{ShowResult: true}}
		s.writeResponse(&contract.ExecutionResponse{Result: contract.ResultSuccess, ExitCode: 0, Message: "message\x1b[2J\x07"})
		plain := themeSGR.ReplaceAllString(out.String(), "")
		if strings.ContainsAny(plain, "\x1b\x07") || !strings.Contains(plain, "message�[2J�") {
			t.Fatal(out.String())
		}
		out.Reset()
		s.writeStyled("31m\x1b[2J", "invalid style\x1b[2J")
		if strings.Contains(out.String(), "\x1b") {
			t.Fatal("untrusted style emitted escapes", out)
		}
	}
}
