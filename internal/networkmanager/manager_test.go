//go:build linux

package networkmanager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"safeterm/internal/contract"
)

const fixture = "# Administrator comment\nnetwork:\n  version: 2\n  renderer: networkd\n  ethernets:\n    ens19:\n      dhcp4: false # important\n      addresses: [192.0.2.10/24]\n      optional: true # retain this exactly\n"

func testTree(t *testing.T) (string, Policy) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"etc/netplan", "etc/safeterm", "run/netplan", "lib/netplan", "state"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", fixture)
	p := DefaultPolicy()
	p.Interfaces = []string{"ens19"}
	return root, p
}
func writeTest(t *testing.T, root, path, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func discoverTest(t *testing.T, root string, p Policy) *Snapshot {
	t.Helper()
	s, e := Discover(root, p, map[string]string{"ens19": "00:11:22:33:44:55"})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestInterfacePolicyDenialExplainsAdministratorFix(t *testing.T) {
	root, p := testTree(t)
	p.Interfaces = nil
	s := discoverTest(t, root, p)
	change := Change{Kind: "interface", Name: "ens19", Field: "addresses", Action: "append", Values: []string{"192.168.222.11/24"}}
	_, err := Modify(s.View, []Change{change})
	if err == nil {
		t.Fatal("unlisted interface was editable")
	}
	for _, want := range []string{"outside administrator policy", `"ens19"`, PolicyPath, "reopen config network interface"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing remediation %q: %v", want, err)
		}
	}
	// A newly opened view uses the changed policy; the previous view stays denied.
	p.Interfaces = []string{"ens19"}
	if _, err := Modify(discoverTest(t, root, p).View, []Change{change}); err != nil {
		t.Fatal(err)
	}
	if _, err := Modify(s.View, []Change{change}); err == nil {
		t.Fatal("cached view silently gained access")
	}
}

func TestReadPolicyUsesGroupedAdministratorPath(t *testing.T) {
	root, p := testTree(t)
	if err := os.Mkdir(filepath.Join(root, "etc/safeterm/network"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "etc/safeterm/network/policy.json", string(data))
	got, err := ReadPolicy(root)
	if err != nil || len(got.Interfaces) != 1 || got.Interfaces[0] != "ens19" {
		t.Fatalf("grouped policy not loaded: %+v %v", got, err)
	}
}
func TestDiscoverAndPatchArbitraryFilename(t *testing.T) {
	root, p := testTree(t)
	s := discoverTest(t, root, p)
	v, err := Modify(s.View, []Change{{Kind: "interface", Name: "ens19", Field: "addresses", Action: "set", Values: []string{"192.0.2.20/24", "192.0.2.21/24"}}, {Kind: "interface", Name: "ens19", Field: "dns", Action: "set", Values: []string{"1.1.1.1"}}, {Kind: "interface", Name: "ens19", Field: "dhcp4", Action: "set", Values: []string{"enable"}}})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := s.Render(v)
	if err != nil {
		t.Fatal(err)
	}
	data := string(changes["etc/netplan/arbitrary-name.yaml"])
	for _, keep := range []string{"# Administrator comment", "# important", "      optional: true # retain this exactly", "192.0.2.20/24", "192.0.2.21/24", "1.1.1.1"} {
		if !strings.Contains(data, keep) {
			t.Fatalf("lost %q:\n%s", keep, data)
		}
	}
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", data)
	after := discoverTest(t, root, p)
	if !after.View.Interfaces["ens19"].DHCP4 || len(after.View.Interfaces["ens19"].Addresses) != 2 {
		t.Fatal(after.View)
	}
}
func TestMergeShadowAndSequenceConsolidation(t *testing.T) {
	root, p := testTree(t)
	writeTest(t, root, "lib/netplan/arbitrary-name.yaml", strings.ReplaceAll(fixture, "192.0.2.10", "192.0.2.99"))
	writeTest(t, root, "etc/netplan/zz-extra.yaml", "network:\n  version: 2\n  ethernets:\n    ens19:\n      addresses: [198.51.100.1/24]\n")
	s := discoverTest(t, root, p)
	if got := s.View.Interfaces["ens19"].Addresses; len(got) != 2 || got[0] != "192.0.2.10/24" {
		t.Fatal(got)
	}
	v, err := Modify(s.View, []Change{{Kind: "interface", Name: "ens19", Field: "addresses", Action: "set", Values: []string{"203.0.113.1/24"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Render(v)
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range changed {
		writeTest(t, root, path, string(data))
	}
	after := discoverTest(t, root, p)
	if got := after.View.Interfaces["ens19"].Addresses; len(got) != 1 || got[0] != "203.0.113.1/24" {
		t.Fatal(got)
	}
	writeTest(t, root, "run/netplan/arbitrary-name.yaml", fixture)
	if discoverTest(t, root, p).View.Interfaces["ens19"].ReadOnly == "" {
		t.Fatal("runtime-owned source editable")
	}
}
func TestRefuseAmbiguousAndUnsafeSources(t *testing.T) {
	for name, content := range map[string]string{"duplicates": "network:\n  version: 2\n  version: 2\n", "anchor": "network: &n\n  version: 2\n", "documents": "network:\n  version: 2\n---\nnetwork:\n  version: 2\n"} {
		t.Run(name, func(t *testing.T) {
			root, p := testTree(t)
			writeTest(t, root, "etc/netplan/arbitrary-name.yaml", content)
			if _, err := Discover(root, p, nil); err == nil {
				t.Fatal("unsafe YAML accepted")
			}
		})
	}
	root, p := testTree(t)
	if err := os.Chmod(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"), 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root, p, nil); err == nil {
		t.Fatal("writable source accepted")
	}
}
func TestFileReplacementPreservesMetadataAndRejectsConflict(t *testing.T) {
	root, _ := testTree(t)
	path := "etc/netplan/arbitrary-name.yaml"
	full := filepath.Join(root, path)
	if err := unix.Setxattr(full, "user.safeterm-test", []byte("keep"), 0); err != nil {
		t.Fatal(err)
	}
	f, err := readTrusted(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = replaceFile(root, f, []byte(fixture+"# end\n")); err != nil {
		t.Fatal(err)
	}
	next, err := readTrusted(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if next.Mode != f.Mode || next.UID != f.UID || next.GID != f.GID || !equal(next.Attrs, f.Attrs) {
		t.Fatal("metadata changed")
	}
	if err = replaceFile(root, f, []byte(fixture)); err == nil {
		t.Fatal("stale overwrite accepted")
	}
}

type fakeTrial struct {
	done     chan struct{}
	once     sync.Once
	accepted bool
}

func (t *fakeTrial) Ready(context.Context) error { return nil }
func (t *fakeTrial) Done() <-chan struct{}       { return t.done }
func (t *fakeTrial) Finish(accept bool) error {
	t.accepted = accept
	t.once.Do(func() { close(t.done) })
	return nil
}

type fakeBackend struct {
	trial    *fakeTrial
	restored int
	invalid  bool
}

func (b *fakeBackend) Validate(context.Context, []File, map[string][]byte) error {
	if b.invalid {
		return errors.New("invalid netplan")
	}
	return nil
}
func (b *fakeBackend) Begin(context.Context, int) (Trial, error) {
	b.trial = &fakeTrial{done: make(chan struct{})}
	return b.trial, nil
}
func (b *fakeBackend) Restore(context.Context) error                    { b.restored++; return nil }
func (b *fakeBackend) Status(context.Context, []string) (string, error) { return "status", nil }
func managerTest(t *testing.T) (*Manager, string, Policy, *fakeBackend) {
	t.Helper()
	root, p := testTree(t)
	b := &fakeBackend{}
	m, err := New(Options{Root: root, State: filepath.Join(root, "state"), Policy: func() (Policy, error) { return p, nil }, Interfaces: func() map[string]string { return map[string]string{"ens19": ""} }, Backend: b})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m, root, p, b
}
func callTest(m *Manager, user string, roles []string, r Request) (Reply, error) {
	data, _ := json.Marshal(r)
	req, err := contract.NewExecutionRequest("network_"+r.Action, map[string]string{"payload": string(data)}, contract.Metadata{BrokerType: "ssh", UserID: user, ClaimedRoles: roles, ReceivedAt: time.Now().UTC()})
	if err != nil {
		return Reply{}, err
	}
	response, err := m.Handle(context.Background(), req)
	if err != nil {
		return Reply{}, err
	}
	var reply Reply
	err = DecodeJSON([]byte(response.Stdout), &reply)
	return reply, err
}
func operatorCall(t *testing.T, m *Manager, r Request) Reply {
	t.Helper()
	v, err := callTest(m, "admin", []string{"network-operator"}, r)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func saveTest(t *testing.T, m *Manager) {
	t.Helper()
	v := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: v.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Field: "addresses", Action: "set", Values: []string{"192.0.2.20/24"}}}})
}
func TestCandidateTrialConfirmAndRollback(t *testing.T) {
	m, root, _, b := managerTest(t)
	saveTest(t, m)
	file := filepath.Join(root, "etc/netplan/arbitrary-name.yaml")
	data, _ := os.ReadFile(file)
	if string(data) != fixture {
		t.Fatal("save changed original")
	}
	operatorCall(t, m, Request{Action: "apply"})
	data, _ = os.ReadFile(file)
	if string(data) != fixture {
		t.Fatal("trial changed original")
	}
	overlay, err := os.ReadFile(filepath.Join(root, "run/netplan/arbitrary-name.yaml"))
	if err != nil || !strings.Contains(string(overlay), "192.0.2.20") {
		t.Fatal(err, string(overlay))
	}
	if _, err = callTest(m, "other", []string{"network-operator"}, Request{Action: "confirm"}); err == nil {
		t.Fatal("another identity confirmed")
	}
	operatorCall(t, m, Request{Action: "confirm"})
	data, _ = os.ReadFile(file)
	if !strings.Contains(string(data), "192.0.2.20") || !b.trial.accepted {
		t.Fatal("not confirmed")
	}
	if _, err = os.Stat(filepath.Join(root, "run/netplan/arbitrary-name.yaml")); !os.IsNotExist(err) {
		t.Fatal("overlay retained")
	}
	operatorCall(t, m, Request{Action: "rollback"})
	data, _ = os.ReadFile(file)
	if string(data) != fixture || b.restored == 0 {
		t.Fatal("rollback failed")
	}
}
func TestTimeoutAndRestartRecovery(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		m, root, _, b := managerTest(t)
		saveTest(t, m)
		operatorCall(t, m, Request{Action: "apply"})
		b.trial.once.Do(func() { close(b.trial.done) })
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			m.mu.Lock()
			pending := m.pending != nil
			m.mu.Unlock()
			if !pending {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.pending != nil || b.restored == 0 {
			t.Fatal("trial not rolled back")
		}
		data, _ := os.ReadFile(filepath.Join(root, "etc/netplan/arbitrary-name.yaml"))
		if string(data) != fixture {
			t.Fatal("persistent source changed")
		}
	})
	t.Run("restart", func(t *testing.T) {
		m, root, p, b := managerTest(t)
		saveTest(t, m)
		operatorCall(t, m, Request{Action: "apply"})
		recovered, err := New(Options{Root: root, State: filepath.Join(root, "state"), Policy: func() (Policy, error) { return p, nil }, Interfaces: func() map[string]string { return map[string]string{"ens19": ""} }, Backend: b})
		if err != nil {
			t.Fatal(err)
		}
		defer recovered.Close()
		if recovered.pending != nil || b.restored == 0 {
			t.Fatal("startup did not recover")
		}
		m.mu.Lock()
		m.pending = nil
		m.trial = nil
		m.mu.Unlock()
	})
}
func TestAuthorizationValidationAndStaleSources(t *testing.T) {
	m, root, _, b := managerTest(t)
	for _, r := range []Request{{Action: "read"}, {Action: "save"}, {Action: "apply"}, {Action: "confirm"}, {Action: "rollback"}} {
		if _, err := callTest(m, "viewer", []string{"viewer"}, r); err == nil {
			t.Fatal("unauthorized", r.Action)
		}
	}
	v := operatorCall(t, m, Request{Action: "read"})
	if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "save", Version: v.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Field: "addresses", Action: "set", Values: []string{"bad"}}}}); err == nil {
		t.Fatal("invalid CIDR accepted")
	}
	saveTest(t, m)
	b.invalid = true
	if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "apply"}); err == nil {
		t.Fatal("invalid netplan trial started")
	}
	if b.trial != nil {
		t.Fatal("backend begin after failed generate")
	}
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", fixture+"# external edit\n")
	if _, err := callTest(m, "admin", []string{"network-operator"}, Request{Action: "apply"}); err == nil {
		t.Fatal("stale candidate applied")
	}
	operatorCall(t, m, Request{Action: "discard"})
}
func TestLockBeforeRecovery(t *testing.T) {
	root, _ := testTree(t)
	lock, err := LockState(root, filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if other, err := LockState(root, filepath.Join(root, "state")); err == nil {
		other.Close()
		t.Fatal("second daemon lock accepted")
	}
}
func TestActualNetplanGenerateOnPrivateRoot(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/netplan"); err != nil {
		t.Skip("netplan.io not installed")
	}
	root, p := testTree(t)
	s := discoverTest(t, root, p)
	v, err := Modify(s.View, []Change{{Kind: "interface", Name: "ens19", Field: "addresses", Action: "set", Values: []string{"192.0.2.25/24"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Render(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = (&Netplan{Root: root}).Validate(context.Background(), s.Files, changed); err != nil {
		t.Fatal(err)
	}
}

func TestMACMatchAndGeneratedFileAcknowledgement(t *testing.T) {
	root, p := testTree(t)
	writeTest(t, root, "etc/netplan/arbitrary-name.yaml", "# cloud-init\nnetwork:\n  version: 2\n  ethernets:\n    uplink:\n      match:\n        macaddress: 00:11:22:33:44:55\n      set-name: ens19\n      dhcp4: true\n")
	s := discoverTest(t, root, p)
	if s.View.Interfaces["ens19"].ReadOnly == "" {
		t.Fatal("generated file became editable implicitly")
	}
	p.AllowGeneratedFiles = true
	s = discoverTest(t, root, p)
	item := s.View.Interfaces["ens19"]
	if item.ID != "uplink" || item.ReadOnly != "" {
		t.Fatal(item)
	}
	v, err := Modify(s.View, []Change{{Kind: "interface", Name: "ens19", Field: "dhcp4", Action: "set", Values: []string{"disable"}}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Render(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changed["etc/netplan/arbitrary-name.yaml"]), "    uplink:\n") {
		t.Fatal("Netplan ID was renamed")
	}
}
func TestJSONPolicyRejectsUnknownDuplicateAndInvalidLimits(t *testing.T) {
	for _, data := range []string{`{"version":1,"version":1}`, `{"version":1,"evil":true}`, `{} {}`} {
		var p Policy
		if err := DecodeJSON([]byte(data), &p); err == nil {
			t.Fatal("accepted", data)
		}
	}
	p := DefaultPolicy()
	p.Interfaces = []string{"../eth0"}
	if p.Validate() == nil {
		t.Fatal("unsafe interface accepted")
	}
}

func TestNetopsHasDefaultNetworkPrivileges(t *testing.T) {
	m, _, _, _ := managerTest(t)
	read, err := callTest(m, "network-user", []string{"netops"}, Request{Action: "read"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = callTest(m, "network-user", []string{"netops"}, Request{Action: "save", Version: read.View.Version, Changes: []Change{{Kind: "interface", Name: "ens19", Field: "mtu", Action: "set", Values: []string{"1400"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"apply", "confirm", "rollback"} {
		if _, err = callTest(m, "network-user", []string{"netops"}, Request{Action: action}); err != nil {
			t.Fatalf("netops %s: %v", action, err)
		}
	}
}

func TestUnsafeSourceReportsSpecificMetadataFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, string)
		want    []string
	}{
		{"writable", func(t *testing.T, path string) {
			if err := os.Chmod(path, 0666); err != nil {
				t.Fatal(err)
			}
		}, []string{"mode=0666", "group/world write"}},
		{"hardlink", func(t *testing.T, path string) {
			if err := os.Link(path, path+".link"); err != nil {
				t.Fatal(err)
			}
		}, []string{"hard-link count=2"}},
		{"oversized", func(t *testing.T, path string) {
			if err := os.Truncate(path, (1<<20)+1); err != nil {
				t.Fatal(err)
			}
		}, []string{"size=1048577 bytes", "maximum is 1048576 bytes"}},
		{"fifo", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := unix.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
		}, []string{"expected a regular file"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := testTree(t)
			relative := "etc/netplan/arbitrary-name.yaml"
			path := filepath.Join(root, relative)
			tc.prepare(t, path)
			_, err := readTrusted(root, relative)
			if err == nil {
				t.Fatal("unsafe file accepted")
			}
			for _, want := range append(tc.want, path) {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing %q: %v", want, err)
				}
			}
		})
	}
}
