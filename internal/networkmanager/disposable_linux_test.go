//go:build linux

package networkmanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This test changes networking ONLY inside an explicitly provisioned nspawn guest.
func TestDisposableNetplanTrial(t *testing.T) {
	if os.Getenv("SAFETERM_NETPLAN_DISPOSABLE") != "1" {
		t.Skip("requires the isolated Ubuntu 24.04 guest")
	}
	container, err := os.ReadFile("/run/systemd/container")
	if err != nil || strings.TrimSpace(string(container)) != "systemd-nspawn" || os.Geteuid() != 0 {
		t.Fatal("refusing to change networking outside a disposable nspawn guest")
	}
	if _, err = os.Stat("/sys/class/net/host0"); err != nil {
		t.Fatal("isolated host0 is absent")
	}
	for _, dir := range []string{"/etc/netplan", "/var/lib/safeterm/network"} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	source := "/etc/netplan/73-an-arbitrary-filename.yaml"
	original := "# retain the administrator's file\nnetwork:\n  version: 2\n  renderer: networkd\n  ethernets:\n    host0:\n      dhcp4: false\n      addresses: [192.0.2.10/24]\n      optional: true # preserve me\n"
	if err = os.WriteFile(source, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err = command(context.Background(), "/usr/sbin/netplan", "apply"); err != nil {
		t.Fatal(err)
	}
	p := DefaultPolicy()
	p.Interfaces = []string{"host0"}
	p.Files = []string{filepath.Base(source)}
	p.ConfirmSeconds = 10
	backend := &Netplan{Root: "/"}
	m, err := New(Options{Root: "/", State: StatePath, Policy: func() (Policy, error) { return p, nil }, Interfaces: PhysicalInterfaces, Backend: backend})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	save := func(address string) {
		v := operatorCall(t, m, Request{Action: "read"})
		operatorCall(t, m, Request{Action: "save", Version: v.View.Version, Changes: []Change{{Kind: "interface", Name: "host0", Field: "addresses", Action: "set", Values: []string{address}}, {Kind: "interface", Name: "host0", Field: "dns", Action: "set", Values: []string{"1.1.1.1", "9.9.9.9"}}, {Kind: "interface", Name: "host0", Field: "mtu", Action: "set", Values: []string{"1400"}}}})
	}
	hasAddress := func(address string) bool {
		data, err := command(context.Background(), "/usr/sbin/ip", "-j", "address", "show", "dev", "host0")
		return err == nil && strings.Contains(data, `"local":"`+address+`"`)
	}
	save("192.0.2.20/24")
	operatorCall(t, m, Request{Action: "apply"})
	if !hasAddress("192.0.2.20") {
		t.Fatal("trial not live")
	}
	data, _ := os.ReadFile(source)
	if string(data) != original {
		t.Fatal("trial touched persistent file")
	}
	operatorCall(t, m, Request{Action: "confirm"})
	data, _ = os.ReadFile(source)
	if !strings.Contains(string(data), "192.0.2.20/24") || !strings.Contains(string(data), "optional: true # preserve me") {
		t.Fatal(string(data))
	}
	info, _ := os.Stat(source)
	if info.Mode().Perm() != 0640 {
		t.Fatal("permissions changed")
	}
	operatorCall(t, m, Request{Action: "rollback"})
	if !hasAddress("192.0.2.10") {
		t.Fatal("confirmed rollback did not restore address")
	}
	save("192.0.2.30/24")
	operatorCall(t, m, Request{Action: "apply"})
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		pending := m.pending != nil
		m.mu.Unlock()
		if !pending {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !hasAddress("192.0.2.10") {
		t.Fatal("timeout did not restore network")
	}
	data, _ = os.ReadFile(source)
	if string(data) != original {
		t.Fatal("timeout changed persistent source")
	}
	operatorCall(t, m, Request{Action: "discard"})
}
