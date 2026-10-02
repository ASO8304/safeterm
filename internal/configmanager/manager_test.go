package configmanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"safeterm/internal/audit"
)

type recordingSink struct {
	mu     sync.Mutex
	events []audit.Event
	fail   bool
}

type fakeActivator struct {
	calls []string
	fail  string
}

func (a *fakeActivator) Run(_ context.Context, _ Actor, operation string) error {
	a.calls = append(a.calls, operation)
	if operation == a.fail {
		return os.ErrPermission
	}
	return nil
}

func (s *recordingSink) Record(_ context.Context, event audit.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return os.ErrPermission
	}
	if err := event.Validate(); err != nil {
		return err
	}
	s.events = append(s.events, event)
	return nil
}

func int64Pointer(value int64) *int64 { return &value }

func testRegistry(path string, separate bool) *Registry {
	return &Registry{Version: "test-v1", Resources: []Resource{{
		ID: "cats-options", Description: "Cat daemon options", Profile: "application", Path: path, Format: FormatINI, Access: AccessReadWrite,
		Fields: []FieldSchema{{Section: "cats", Key: "Cat_Path", Type: FieldPath, Required: true, Path: &PathPolicy{AllowedPrefixes: []string{"/etc"}}}, {Section: "cats", Key: "Cat_Port", Type: FieldInteger, Required: true, Min: int64Pointer(1), Max: int64Pointer(65535)}, {Section: "cats", Key: "Password", Type: FieldString, Required: true, Secret: true}, {Section: "cats", Key: "Immutable", Type: FieldString, Required: true, ReadOnly: true}},
		Backup: BackupPolicy{Enabled: true, Keep: 2}, RequireSeparateApproval: separate,
		Permissions: Permissions{ReadRoles: []string{"viewer"}, ProposeRoles: []string{"operator"}, ApplyRoles: []string{"operator", "approver"}, RollbackRoles: []string{"operator"}},
	}}}
}

func writeTestINI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "options.ini")
	data := "; comment\n[cats]\nCat_Path = \"/etc/cat\"\nCat_Port = \"99\"\nPassword = \"hidden\"\nImmutable = \"fixed\"\n"
	if err := os.WriteFile(path, []byte(data), 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}
func actor(id string, roles ...string) Actor {
	return Actor{BrokerType: "ssh", UserID: id, Roles: roles}
}

func TestManagerPreviewApplyRedactionConflictAndRollback(t *testing.T) {
	path := writeTestINI(t)
	sink := &recordingSink{}
	manager, err := NewManager("application", testRegistry(path, false), nil, nil, sink, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	operator := actor("alice", "operator", "viewer")
	read, err := manager.Read(context.Background(), operator, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range read.Fields {
		if field.Key == "Password" && field.Value != "[REDACTED]" {
			t.Fatalf("secret leaked: %#v", field)
		}
	}
	patch := Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "0100", BaseVersion: read.Version}
	validation, err := manager.ValidatePatch(context.Background(), operator, patch)
	if err != nil || !validation.Valid || validation.NormalizedValue != "100" {
		t.Fatalf("validation=%+v err=%v", validation, err)
	}
	preview, err := manager.Preview(context.Background(), operator, patch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview.Diff, "hidden") {
		t.Fatal("secret appeared in diff")
	}
	applied, err := manager.Apply(context.Background(), operator, preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Version == read.Version {
		t.Fatal("version did not change")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Cat_Port = \"100\"") {
		t.Fatalf("unexpected content: %s", data)
	}
	if _, err := manager.Apply(context.Background(), operator, preview.Token); err == nil {
		t.Fatal("proposal replay was accepted")
	}
	rollback, err := manager.Rollback(context.Background(), operator, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Version != read.Version {
		t.Fatalf("rollback version=%s want=%s", rollback.Version, read.Version)
	}

	stale, err := manager.Preview(context.Background(), operator, Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "101", BaseVersion: read.Version})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "100", "102", 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(context.Background(), operator, stale.Token); err == nil || !strings.Contains(err.Error(), "version conflict") {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestManagerRejectsUnknownReadOnlyRangeUnauthorizedAndCrossActor(t *testing.T) {
	path := writeTestINI(t)
	manager, err := NewManager("application", testRegistry(path, false), nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	operator := actor("alice", "operator", "viewer")
	read, _ := manager.Read(context.Background(), operator, "cats-options")
	for _, patch := range []Patch{{ResourceID: "cats-options", Section: "cats", Key: "Nope", Value: "1", BaseVersion: read.Version}, {ResourceID: "cats-options", Section: "cats", Key: "Immutable", Value: "change", BaseVersion: read.Version}, {ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "70000", BaseVersion: read.Version}} {
		result, err := manager.ValidatePatch(context.Background(), operator, patch)
		if err != nil || result.Valid {
			t.Fatalf("patch %#v result=%+v err=%v", patch, result, err)
		}
	}
	if _, err := manager.ValidatePatch(context.Background(), actor("mallory", "viewer"), Patch{ResourceID: "cats-options"}); err == nil {
		t.Fatal("unauthorized proposal was not rejected")
	}
	preview, err := manager.Preview(context.Background(), operator, Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: read.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(context.Background(), actor("bob", "operator"), preview.Token); err == nil || !strings.Contains(err.Error(), "bound") {
		t.Fatalf("cross-actor apply: %v", err)
	}
}

func TestProposalExpiryAndSeparateApproval(t *testing.T) {
	path := writeTestINI(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	manager, err := NewManager("application", testRegistry(path, true), nil, nil, &recordingSink{}, ManagerOptions{ProposalTTL: time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	proposer := actor("alice", "operator", "viewer")
	read, _ := manager.Read(context.Background(), proposer, "cats-options")
	patch := Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: read.Version}
	preview, err := manager.Preview(context.Background(), proposer, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(context.Background(), proposer, preview.Token); err == nil {
		t.Fatal("self approval accepted")
	}
	if _, err := manager.Apply(context.Background(), actor("bob", "approver"), preview.Token); err != nil {
		t.Fatal(err)
	}
	read, _ = manager.Read(context.Background(), proposer, "cats-options")
	preview, _ = manager.Preview(context.Background(), proposer, Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "101", BaseVersion: read.Version})
	now = now.Add(2 * time.Minute)
	if _, err := manager.Apply(context.Background(), actor("bob", "approver"), preview.Token); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expiry error=%v", err)
	}
}

func TestAuditFailurePreventsApply(t *testing.T) {
	path := writeTestINI(t)
	sink := &recordingSink{}
	manager, _ := NewManager("application", testRegistry(path, false), nil, nil, sink, ManagerOptions{})
	a := actor("alice", "operator", "viewer")
	read, _ := manager.Read(context.Background(), a, "cats-options")
	preview, _ := manager.Preview(context.Background(), a, Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: read.Version})
	sink.fail = true
	if _, err := manager.Apply(context.Background(), a, preview.Token); err == nil {
		t.Fatal("apply succeeded without audit")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "100") {
		t.Fatal("file changed after audit rejection")
	}
}

func TestValidatePathRejectsTraversalAndSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	policy := PathPolicy{AllowedPrefixes: []string{directory}, MustExist: true, FileType: "file"}
	if _, err := validatePath(policy, link); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err := validatePath(policy, filepath.Join(directory, "..", "outside")); err == nil {
		t.Fatal("unclean path accepted")
	}
}

func TestActivationHealthFailureRollsBack(t *testing.T) {
	path := writeTestINI(t)
	registry := testRegistry(path, false)
	resource := &registry.Resources[0]
	resource.ActivationOperation = "reload-cats"
	resource.HealthCheckOperation = "check-cats"
	resource.RollbackOnFailure = true
	resource.Permissions.ActivateRoles = []string{"operator"}
	activator := &fakeActivator{fail: "check-cats"}
	manager, err := NewManager("application", registry, nil, activator, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := actor("alice", "viewer", "operator")
	read, _ := manager.Read(context.Background(), a, "cats-options")
	preview, _ := manager.Preview(context.Background(), a, Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: read.Version})
	result, err := manager.Apply(context.Background(), a, preview.Token)
	if err == nil {
		t.Fatal("health failure was ignored")
	}
	if result.RollbackOutcome != "succeeded" || result.Version != read.Version {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
