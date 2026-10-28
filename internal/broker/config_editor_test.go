package broker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/audit"
	"safeterm/internal/configmanager"
)

type editorAudit struct{}

func (editorAudit) Record(_ context.Context, e audit.Event) error { return e.Validate() }

func editorFixture(t *testing.T) (*Shell, string, *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "options.ini")
	data := "; preserve this comment\r\n[Server] ; server settings\r\nPort = 8080 ; port comment\r\nPassword = \"secret-sentinel-editor\"\r\n[Logging]\r\nLevel = info\r\n"
	if err := os.WriteFile(path, []byte(data), 0640); err != nil {
		t.Fatal(err)
	}
	min, max := int64(1), int64(65535)
	r := configmanager.Resource{ID: "options", Profile: "application", Path: path, Format: configmanager.FormatINI, Access: configmanager.AccessReadWrite, Description: "Application settings", Fields: []configmanager.FieldSchema{{Section: "Server", Key: "Port", Type: configmanager.FieldInteger, Min: &min, Max: &max, Description: "Listening port"}, {Section: "Server", Key: "Password", Type: configmanager.FieldString, Secret: true}, {Section: "Logging", Key: "Level", Type: configmanager.FieldEnum, Enum: []string{"info", "debug"}}}, Permissions: configmanager.Permissions{ReadRoles: []string{"viewer"}, ProposeRoles: []string{"operator"}, ApplyRoles: []string{"operator"}, RollbackRoles: []string{"operator"}}, Backup: configmanager.BackupPolicy{Enabled: true, Keep: 2}}
	manager, err := configmanager.NewManager("application", &configmanager.Registry{Version: "test", Resources: []configmanager.Resource{r}}, nil, nil, editorAudit{}, configmanager.ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	configs, _ := NewConfigBroker([]configmanager.Service{manager})
	catalog, _ := NewCatalog(brokerTestConfig())
	out := &bytes.Buffer{}
	shell, err := NewShellWithConfig(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"viewer", "operator"}}, strings.NewReader(""), out, configs)
	if err != nil {
		t.Fatal(err)
	}
	return shell, path, out
}
func editorCommands(t *testing.T, s *Shell, commands ...string) {
	t.Helper()
	for _, c := range commands {
		exit, err := s.executeLine(context.Background(), c, nil)
		if exit || err != nil {
			t.Fatalf("%q: exit=%v err=%v", c, exit, err)
		}
	}
}

func TestConfigEditorStagesAcrossSectionsAndCommitsOnce(t *testing.T) {
	s, path, out := editorFixture(t)
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config options", "edit Server", "set Port 9090", "set Port 9091", "next", "edit Logging", "set Level debug", "diff", "show")
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, before) {
		t.Fatal("staging wrote file")
	}
	if !strings.Contains(s.prompt(), "(options/Logging)") {
		t.Fatal(s.prompt())
	}
	if !strings.Contains(out.String(), "9091") || strings.Contains(out.String(), "secret-sentinel-editor") {
		t.Fatal("incorrect or secret output")
	}
	editorCommands(t, s, "save")
	data, _ = os.ReadFile(path)
	expected := strings.Replace(strings.Replace(string(before), "8080", "9091", 1), "Level = info", "Level = debug", 1)
	if string(data) != expected || s.editing != nil {
		t.Fatalf("unexpected save: %q", data)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".safeterm-*.bak"))
	if len(backups) != 1 {
		t.Fatalf("backups=%v", backups)
	}
	editorCommands(t, s, "config options")
	input := make(chan byte, 2)
	input <- 'y'
	input <- '\r'
	if _, err := s.executeLine(context.Background(), "rollback", input); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !bytes.Equal(data, before) {
		t.Fatal("rollback failed")
	}
}

func TestConfigEditorCancelConflictAndHelpAreSafe(t *testing.T) {
	s, path, out := editorFixture(t)
	ctx := context.Background()
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config options", "?", "edit ?", "edit Server", "set ?", "set Port ?")
	if !strings.Contains(out.String(), "Listening port") {
		t.Fatal("missing field description")
	}
	if got := s.completionCandidates(ctx, "set P", "P"); strings.Join(got, ",") != "Password,Port" {
		t.Fatal(got)
	}
	if _, err := s.executeLine(ctx, "set Port 0", nil); err == nil {
		t.Fatal("invalid range accepted")
	}
	editorCommands(t, s, "set Port 9000", "cancel")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, before) {
		t.Fatal("cancel wrote file")
	}
	editorCommands(t, s, "config options", "edit Server", "set Port 9000")
	external := append(append([]byte(nil), before...), []byte("; external\r\n")...)
	_ = os.WriteFile(path, external, 0640)
	if _, err := s.executeLine(ctx, "save", nil); err == nil {
		t.Fatal("stale end accepted")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(after, external) || s.editing == nil {
		t.Fatal("conflict lost data or draft")
	}
	editorCommands(t, s, "cancel")
}

func TestConfigEditorSecretsUsePrivatePromptAndNeverHistory(t *testing.T) {
	s, path, out := editorFixture(t)
	editorCommands(t, s, "config options", "edit Server")
	input := make(chan byte, 128)
	for _, b := range []byte("my-private-value\r") {
		input <- b
	}
	if _, err := s.executeLine(context.Background(), "set Password", input); err != nil {
		t.Fatal(err)
	}
	editorCommands(t, s, "show", "diff", "save")
	if strings.Contains(out.String(), "my-private-value") {
		t.Fatal("secret echoed")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("my-private-value")) {
		t.Fatal("secret not saved")
	}
	for _, b := range []byte("set Password secret-in-history\r") {
		input <- b
	}
	_, err := s.readLine(context.Background(), input, s.prompt(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.history) != 0 {
		t.Fatal("set command stored in history")
	}
}

func TestOperationTreeKeepsSignedIDsAndFiltersRoles(t *testing.T) {
	cfg := brokerTestConfig()
	for i := range cfg.Operations {
		if cfg.Operations[i].Name == "system_info" {
			cfg.Operations[i].Name = "status_safeterm_ssh"
		}
	}
	for i := range cfg.Policies {
		for j := range cfg.Policies[i].Operations {
			if cfg.Policies[i].Operations[j] == "system_info" {
				cfg.Policies[i].Operations[j] = "status_safeterm_ssh"
			}
		}
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGateway{}
	out := &bytes.Buffer{}
	s, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), out)
	editorCommands(t, s, "execute service ?", "execute service status ?", "execute service status ssh ?", "execute service status ssh")
	if len(client.requests) != 1 || client.requests[0].Command != "status_safeterm_ssh" {
		t.Fatalf("requests=%+v", client.requests)
	}
	for _, tc := range []struct{ before, prefix, want string }{{"execute s", "s", "service"}, {"execute service st", "st", "status"}, {"execute service status s", "s", "ssh"}} {
		if got := s.completionCandidates(context.Background(), tc.before, tc.prefix); strings.Join(got, ",") != tc.want {
			t.Fatalf("%s: %v", tc.before, got)
		}
	}
	if got := s.completionCandidates(context.Background(), "execute cap", "cap"); len(got) != 0 {
		t.Fatal("unauthorized completion")
	}
}

func TestRootDiscoveryUsesMinimalMenuAndConfigListsFiles(t *testing.T) {
	s, _, out := editorFixture(t)
	editorCommands(t, s, "?", "config ?")
	text := out.String()
	for _, name := range []string{"execute", "config", "whoami", "clear", "exit", "options"} {
		if !strings.Contains(text, name) {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"describe NAME", "config apply", "proposal:"} {
		if strings.Contains(text, name) {
			t.Fatalf("redundant menu %s", name)
		}
	}
	if s.editing != nil {
		t.Fatal("help entered edit mode")
	}
}

func TestSystemInfoGroupRequiresAuthorizedSubcommand(t *testing.T) {
	catalog, err := NewCatalog(brokerTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGateway{}
	s, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), &bytes.Buffer{})
	if _, err = s.executeLine(context.Background(), "execute system_info", nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 0 {
		t.Fatal("group dispatched without a subcommand")
	}
	for _, line := range []string{"execute system", "execute memory", "execute disk"} {
		if _, err = s.executeLine(context.Background(), line, nil); err == nil {
			t.Fatalf("removed root command accepted: %s", line)
		}
	}
	if len(client.requests) != 0 {
		t.Fatal("removed root command dispatched")
	}
}
