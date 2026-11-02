//go:build linux

package networkmanager

import (
	"context"
	"net"
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
	mac, err := os.ReadFile("/sys/class/net/host0/address")
	if err != nil {
		t.Fatal(err)
	}
	// The isolated guest deliberately models hardware with veths. Production
	// discovery excludes virtual links; inject this explicit test hardware set.
	hardware := map[string]string{"host0": strings.TrimSpace(string(mac))}
	m, err := New(Options{Root: "/", State: StatePath, Policy: func() (Policy, error) { return p, nil }, Interfaces: func() map[string]string { return hardware }, Backend: backend})
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

	// Hotplug an additional simulated LAN module in this guest only. Confirm
	// that its persistent file is created only after a successful live trial.
	if _, err := command(context.Background(), "/usr/sbin/ip", "link", "add", "future0", "type", "veth", "peer", "name", "future-peer"); err != nil {
		t.Fatal(err)
	}
	defer command(context.Background(), "/usr/sbin/ip", "link", "delete", "future0")
	if _, err := command(context.Background(), "/usr/sbin/ip", "link", "set", "future-peer", "up"); err != nil {
		t.Fatal(err)
	}
	nic, err := net.InterfaceByName("future0")
	if err != nil {
		t.Fatal(err)
	}
	hardware["future0"] = nic.HardwareAddr.String()
	p.AllowAllInterfaces = true
	p.Files = append(p.Files, filepath.Base(managedInterfaces))
	v := operatorCall(t, m, Request{Action: "read"})
	operatorCall(t, m, Request{Action: "save", Version: v.View.Version, Changes: []Change{{Kind: "interface", Name: "future0", Field: "addresses", Action: "set", Values: []string{"198.51.100.10/24"}}}})
	operatorCall(t, m, Request{Action: "apply"})
	if _, err := os.Stat("/" + managedInterfaces); !os.IsNotExist(err) {
		t.Fatal("new hardware trial wrote persistent files", err)
	}
	operatorCall(t, m, Request{Action: "confirm"})
	if data, err := os.ReadFile("/" + managedInterfaces); err != nil || !strings.Contains(string(data), nic.HardwareAddr.String()) {
		t.Fatal("confirmed MAC-matched source missing", string(data), err)
	}
	operatorCall(t, m, Request{Action: "rollback"})
	if _, err := os.Stat("/" + managedInterfaces); !os.IsNotExist(err) {
		t.Fatal("rollback did not remove the created source", err)
	}
	if data, err := command(context.Background(), "/usr/sbin/ip", "-j", "address", "show", "dev", "future0"); err != nil || strings.Contains(data, `"local":"198.51.100.10"`) {
		t.Fatal("new-interface rollback retained the trial address", data, err)
	}

	// Exercise persistent naming under the production filesystem restrictions,
	// reusing an administrator's existing MAC rule without renaming host0 live.
	if err := prepareNamingDirectory("/"); err != nil {
		t.Fatal(err)
	}
	rulePath := "/etc/systemd/network/10-persistent-net.link"
	rule := "# Keep this comment\n[Match]\nMACAddress=" + hardware["host0"] + "\n\n[Link]\nName=host0\n"
	if err := os.WriteFile(rulePath, []byte(rule), 0640); err != nil {
		t.Fatal(err)
	}
	persistNameTest(t, m, "host0", "vmnic0")
	if _, err := net.InterfaceByName("host0"); err != nil || !hasAddress("192.0.2.10") {
		t.Fatal("persistent rename changed the live connection", err)
	}
	if data, err := os.ReadFile(rulePath); err != nil || !strings.Contains(string(data), "Name=vmnic0") || !strings.Contains(string(data), "# Keep this comment") {
		t.Fatal("existing rule was not updated in place", string(data), err)
	}
}
