package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/terminalconfig"
)

func TestPasswordPromptsHideSecretsBindIdentityAndAvoidHistory(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	out := &bytes.Buffer{}
	s, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), out)
	calls := 0
	s.SetPasswordChanger(func(_ context.Context, p Principal, old, next []byte) (Principal, error) {
		calls++
		if p.UserID != "alice" || p.BrokerType != "ssh" || string(old) != "OldPassword42" || string(next) != "NewPassword43" {
			t.Fatal("wrong identity or passwords")
		}
		return p, nil
	})
	for _, command := range []string{"passwd", "change_pass"} {
		exit, err := s.executeLine(context.Background(), command, guideInput("OldPassword42\rNewPassword43\rNewPassword43\r"))
		if err != nil || exit {
			t.Fatal(err)
		}
	}
	if calls != 2 || !strings.Contains(out.String(), "Password changed successfully") {
		t.Fatal("password callback missing")
	}
	for _, value := range []string{"OldPassword42", "NewPassword43"} {
		if strings.Contains(out.String(), value) || strings.Contains(strings.Join(s.history, ","), value) {
			t.Fatal("secret leaked")
		}
	}
	_, err := s.executeLine(context.Background(), "passwd", guideInput("OldPassword42\rNewPassword43\rDifferent44\r"))
	if err == nil || calls != 2 {
		t.Fatal("mismatched confirmation reached service")
	}
	for _, command := range []string{"passwd alice", "passwd OldPassword42 NewPassword43", "whoami"} {
		if _, err := s.executeLine(context.Background(), command, nil); err == nil {
			t.Fatal("unsafe or removed command accepted", command)
		}
	}
}

func TestSSHPasswordReloadTakesEffectWithoutRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	write := func(password string) {
		t.Helper()
		hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordHashCost)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(SSHIdentityConfig{Users: []SSHUser{{UserID: "alice", Roles: []string{"viewer"}, PasswordHash: string(hash)}}})
		if err = os.WriteFile(path, data, 0640); err != nil {
			t.Fatal(err)
		}
	}
	write("OldPassword42")
	a, err := LoadSSHAuthenticator(path, map[string]bool{"viewer": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.AuthenticatePassword(connectionMetadata{user: "alice"}, []byte("OldPassword42")); err != nil {
		t.Fatal(err)
	}
	write("NewPassword43")
	if _, err = a.AuthenticatePassword(connectionMetadata{user: "alice"}, []byte("OldPassword42")); err == nil {
		t.Fatal("old password remains accepted")
	}
	if _, err = a.AuthenticatePassword(connectionMetadata{user: "alice"}, []byte("NewPassword43")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`{"users":[],"unexpected":true}`), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err = a.AuthenticatePassword(connectionMetadata{user: "alice"}, []byte("NewPassword43")); err == nil {
		t.Fatal("unsafe reload fell back to stale hash")
	}
}

func TestQuietOperationOutcomesKeepRealFailuresVisible(t *testing.T) {
	for _, tc := range []struct {
		message, stage       string
		code                 int
		cancel, probe, debug bool
		want                 string
	}{
		{"execution: canceled: semantic terminate requested", "execution", -1, true, false, false, "Operation stopped."},
		{"", "execution", 130, true, false, false, "Operation stopped."},
		{"audit failed", "execution", -1, true, false, false, "Error: Command failed."},
		{"exceeded output limit", "execution", -1, true, false, false, "Error: Command failed."},
		{"execution: port probe: Connection refused.", "execution", 1, false, true, false, ""},
		{"audit failed", "execution", 1, false, true, false, "Error: Command failed."},
		{"execution: port probe: Connection refused.", "execution", 1, false, true, true, "result: execution_error exit=1"},
	} {
		out := &bytes.Buffer{}
		s := &Shell{output: out}
		_ = s.SetPresentation(terminalconfig.Config{ShowResult: tc.debug})
		probe := ""
		if tc.probe {
			probe = "Trying 192.0.2.1:443...\nConnection refused.\n"
		}
		s.writeOperationResponse(&contract.ExecutionResponse{Result: contract.ResultExecutionError, Message: tc.message, FailureStage: tc.stage, ExitCode: tc.code}, tc.cancel, probe)
		if tc.want == "" && out.Len() != 0 || tc.want != "" && !strings.Contains(out.String(), tc.want) {
			t.Fatal(tc, out.String())
		}
	}
}

func TestCapturePairsKeepSignedFilterValuesAndHideQuestionCompletion(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := NewCatalog(cfg)
	s, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, strings.NewReader(""), &bytes.Buffer{})
	op, _ := catalog.AuthorizedOperation("capture", s.principal.Roles)
	for _, input := range [][]string{{"host", "192.0.2.1", "port", "53", "protocol", "udp"}, {"host=192.0.2.1", "port=53", "protocol=udp"}} {
		args, err := parseOperationArguments(op, input)
		if err != nil || !reflect.DeepEqual(args, map[string]string{"host": "192.0.2.1", "port": "53", "protocol": "udp"}) {
			t.Fatal(args, err)
		}
	}
	for _, input := range [][]string{{"host"}, {"host", "port", "53"}, {"host", "192.0.2.1", "host", "192.0.2.2"}, {"--listen", "123"}} {
		if _, err := parseOperationArguments(op, input); err == nil {
			t.Fatal("invalid filter syntax accepted", input)
		}
	}
	for _, choice := range s.completionCandidates(context.Background(), "", "") {
		if choice == "?" || choice == "whoami" {
			t.Fatal("hidden command advertised", choice)
		}
	}
}
