package broker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/configmanager"
)

func logicalConfigFixture(t *testing.T) (*Shell, string, *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shared.ini")
	data := "\ufeff; keep comment\r\n[Tunnel]\r\nTCPMSS = 1360 ; preserve\r\nPassword = secret-sentinel\r\n[Cat]\r\nPort_2 = 2420\r\nEnabled = true\r\nUnmanaged = untouched\r\n"
	if err := os.WriteFile(path, []byte(data), 0640); err != nil {
		t.Fatal(err)
	}
	min, max := int64(1), int64(65535)
	r := configmanager.Resource{ID: "physical-file", Profile: "application", Path: path, Format: configmanager.FormatINI, Access: configmanager.AccessReadWrite, Description: "Shared settings", Backup: configmanager.BackupPolicy{Enabled: true, Keep: 5}, Permissions: configmanager.Permissions{ReadRoles: []string{"viewer", "operator"}, ProposeRoles: []string{"operator"}, ApplyRoles: []string{"operator"}, RollbackRoles: []string{"operator"}}, Fields: []configmanager.FieldSchema{
		{Section: "Tunnel", Key: "TCPMSS", Type: configmanager.FieldInteger, Min: &min, Max: &max},
		{Section: "Cat", Key: "Port_2", Type: configmanager.FieldPort},
		{Section: "Tunnel", Key: "Password", Type: configmanager.FieldString, Secret: true},
		{Section: "Cat", Key: "Enabled", Type: configmanager.FieldBoolean},
	}, Contexts: []configmanager.ConfigContext{
		{Path: []string{"tunnel"}, Fields: []configmanager.FieldAlias{{Name: "Value_1", Section: "Tunnel", Key: "TCPMSS"}, {Name: "Value_2", Section: "Cat", Key: "Port_2"}, {Name: "token", Section: "Tunnel", Key: "Password"}}},
		{Path: []string{"application", "cat"}, Fields: []configmanager.FieldAlias{{Name: "enabled", Section: "Cat", Key: "Enabled"}}},
	}}
	manager, err := configmanager.NewManager("application", &configmanager.Registry{Version: "test", Resources: []configmanager.Resource{r}}, nil, nil, editorAudit{}, configmanager.ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	configs, err := NewConfigBroker([]configmanager.Service{manager})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(brokerTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	shell, err := NewShellWithConfig(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"viewer", "operator"}}, strings.NewReader(""), out, configs)
	if err != nil {
		t.Fatal(err)
	}
	return shell, path, out
}

func TestLogicalConfigMappingAndSharedFilePreservation(t *testing.T) {
	s, path, out := logicalConfigFixture(t)
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config tunnel", "show", "set Value_1 1400", "set Value_2 443", "show", "diff")
	for _, name := range []string{"TCPMSS", "Port_2", "secret-sentinel", "physical-file"} {
		if strings.Contains(out.String(), name) {
			t.Fatalf("physical or secret detail exposed: %s", out)
		}
	}
	if !strings.Contains(out.String(), "Value_1") || !strings.Contains(s.prompt(), "(tunnel)") {
		t.Fatal("logical names missing")
	}
	for _, command := range []string{"set TCPMSS 1500", "set Port_2 80", "set enabled false", "edit Tunnel", "set Value_2 65536"} {
		if _, err := s.executeLine(context.Background(), command, nil); err == nil {
			t.Fatalf("accepted invalid logical command %q", command)
		}
	}
	current, _ := os.ReadFile(path)
	if string(current) != string(before) {
		t.Fatal("draft wrote before end")
	}
	editorCommands(t, s, "save")
	expected := strings.Replace(strings.Replace(string(before), "TCPMSS = 1360", "TCPMSS = 1400", 1), "Port_2 = 2420", "Port_2 = 443", 1)
	current, _ = os.ReadFile(path)
	if string(current) != expected {
		t.Fatalf("unexpected document %q", current)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".safeterm-*.bak"))
	if len(backups) != 1 {
		t.Fatalf("backups=%v", backups)
	}
	editorCommands(t, s, "config application cat", "set enabled false", "save")
	current, _ = os.ReadFile(path)
	if string(current) != strings.Replace(expected, "Enabled = true", "Enabled = false", 1) {
		t.Fatalf("second view lost first save: %s", current)
	}
}

func TestLogicalConfigDiscoveryPermissionsAndCancellation(t *testing.T) {
	s, path, out := logicalConfigFixture(t)
	for _, tc := range []struct{ before, prefix, want string }{
		{"config ", "", "application,tunnel"}, {"config application ", "", "cat"}, {"config t", "t", "tunnel"},
	} {
		if got := strings.Join(s.completionCandidates(context.Background(), tc.before, tc.prefix), ","); got != tc.want {
			t.Errorf("%q = %s", tc.before, got)
		}
	}
	editorCommands(t, s, "config ?", "config application ?", "config tunnel ?")
	if s.editing != nil || !strings.Contains(out.String(), "Value_1") || strings.Contains(out.String(), "TCPMSS") {
		t.Fatalf("help is not logical: %s", out)
	}
	if _, err := s.executeLine(context.Background(), "config physical-file", nil); err == nil {
		t.Fatal("hidden physical ID exposed")
	}
	editorCommands(t, s, "config tunnel")
	if got := strings.Join(s.completionCandidates(context.Background(), "set V", "V"), ","); got != "Value_1,Value_2" {
		t.Fatalf("field completion %s", got)
	}
	editorCommands(t, s, "set Value_1 1401", "cancel")
	original, _ := os.ReadFile(path)
	editorCommands(t, s, "config tunnel", "set Value_1 1401")
	external := strings.Replace(string(original), "Unmanaged = untouched", "Unmanaged = external", 1)
	if err := os.WriteFile(path, []byte(external), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := s.executeLine(context.Background(), "save", nil); err == nil || s.editing == nil {
		t.Fatal("stale save accepted or draft lost")
	}
	current, _ := os.ReadFile(path)
	if string(current) != external {
		t.Fatal("stale save overwrote external edit")
	}
	editorCommands(t, s, "cancel")
	s.principal.Roles = []string{"viewer"}
	editorCommands(t, s, "config tunnel", "set Value_1 1450")
	if _, err := s.executeLine(context.Background(), "save", nil); err == nil {
		t.Fatal("read-only principal saved")
	}
	editorCommands(t, s, "cancel")
	s.principal.Roles = []string{"netops"}
	if got := s.completionCandidates(context.Background(), "config ", ""); len(got) != 0 {
		t.Fatalf("unauthorized contexts %v", got)
	}
	if _, err := s.executeLine(context.Background(), "config tunnel", nil); err == nil {
		t.Fatal("unauthorized context opened")
	}
}
