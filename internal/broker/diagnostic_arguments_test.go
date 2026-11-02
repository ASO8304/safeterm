package broker

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
)

func TestDiagnosticPositionalSyntaxAndCountBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields []string
		want   map[string]string
	}{
		{"ping", []string{"192.0.2.1"}, map[string]string{"host": "192.0.2.1"}},
		{"ping", []string{"192.0.2.1", "-c", "5"}, map[string]string{"host": "192.0.2.1", "count": "5"}},
		{"ping", []string{"-c", "1000", "192.0.2.1"}, map[string]string{"host": "192.0.2.1", "count": "1000"}},
		{"telnet", []string{"192.0.2.1"}, map[string]string{"host": "192.0.2.1"}},
		{"telnet", []string{"::1", "443"}, map[string]string{"host": "::1", "port": "443"}},
		{"udp-test", []string{"192.0.2.1", "53"}, map[string]string{"host": "192.0.2.1", "port": "53"}},
		{"udp-test", []string{"::1", "123"}, map[string]string{"host": "::1", "port": "123"}},
	} {
		got, err := parseOperationArguments(config.OperationDefinition{Name: tc.name}, tc.fields)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc, got, err)
		}
	}
	for _, fields := range [][]string{{"192.0.2.1", "-c", "1100"}, {"192.0.2.1", "-c", "0"}, {"192.0.2.1", "-c", "-1"}, {"192.0.2.1", "-c"}, {"192.0.2.1", "-c", "5", "-c", "10"}, {"host=192.0.2.1"}, {"192.0.2.1", "count=5"}, {"192.0.2.1;id"}, {"192.0.2.1", "192.0.2.2"}} {
		if _, err := parseOperationArguments(config.OperationDefinition{Name: "ping"}, fields); err == nil {
			t.Fatal("accepted", fields)
		}
	}
	if _, err := parseOperationArguments(config.OperationDefinition{Name: "ping"}, []string{"192.0.2.1", "-c", "1100"}); err == nil || !strings.Contains(err.Error(), "between 1 and 1000") {
		t.Fatal(err)
	}
}

func TestUDPDiagnosticUsesRequiredPortAndAuthorizedNetcatOperation(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGateway{}
	out := &bytes.Buffer{}
	s, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, strings.NewReader(""), out)
	if got := s.completionCandidates(context.Background(), "execute udp", "udp"); !reflect.DeepEqual(got, []string{"udp-test"}) {
		t.Fatal(got)
	}
	editorCommands(t, s, "execute udp-test ?")
	for _, want := range []string{"nc -zu", "Required UDP port", "inconclusive", "5 seconds"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("UDP help missing", want, out)
		}
	}
	if len(client.requests) != 0 {
		t.Fatal("UDP help executed a scan")
	}
	for _, args := range []string{"127.0.0.1", "example.com 53", "127.0.0.1 1-65535", "127.0.0.1 53 --script=all", "127.0.0.1 0", "127.0.0.1 65536"} {
		if _, err := s.executeLine(context.Background(), "execute udp-test "+args, nil); err == nil || len(client.requests) != 0 {
			t.Fatal("invalid UDP input reached gateway", args, err)
		}
	}
	input := make(chan byte, 1)
	input <- 0x03
	if exit, err := s.executeLine(context.Background(), "execute udp-test 127.0.0.1 53", input); exit || err != nil || len(client.terminated) != 1 {
		t.Fatal("UDP Ctrl+C did not stop only the operation", exit, err)
	}
	if client.requests[0].Command != "udp-test" || client.requests[0].Arguments["port"] != "53" {
		t.Fatal(client.requests)
	}
	editorCommands(t, s, "whoami")
	viewer, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), &bytes.Buffer{})
	if _, err := viewer.executeLine(context.Background(), "execute udp-test 127.0.0.1 53", nil); err == nil || len(client.requests) != 1 {
		t.Fatal("viewer ran a UDP scan", err)
	}
}
func TestDiagnosticHelpIsReadOnlyAndRoleFiltered(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	client := &fakeGateway{}
	s, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, strings.NewReader(""), out)
	editorCommands(t, s, "execute ping ?", "execute ping 192.0.2.1 -c ?", "execute telnet ?")
	for _, want := range []string{"1..1000", "default 1000", "default 23", "5 seconds", "Ctrl+C"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("missing", want, out.String())
		}
	}
	if len(client.requests) != 0 {
		t.Fatal("help executed network operation")
	}
	viewer, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), &bytes.Buffer{})
	if _, err := viewer.executeLine(context.Background(), "execute telnet 127.0.0.1 23", nil); err == nil {
		t.Fatal("viewer ran telnet")
	}
}

func TestTelnetCtrlCStopsOnlyOperation(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	gateway := &fakeGateway{}
	out := &bytes.Buffer{}
	s, _ := NewShell(gateway, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, strings.NewReader(""), out)
	input := make(chan byte, 1)
	input <- 0x03
	exit, err := s.executeLine(context.Background(), "execute telnet 127.0.0.1 23", input)
	if err != nil || exit || len(gateway.terminated) != 1 {
		t.Fatal(exit, err, gateway.terminated)
	}
	editorCommands(t, s, "whoami")
	if !strings.Contains(out.String(), "user: alice") {
		t.Fatal("SafeTerm prompt was not usable after Ctrl+C")
	}
	if gateway.requests[0].Command != "telnet" || gateway.requests[0].Arguments["host"] != "127.0.0.1" || gateway.requests[0].Arguments["port"] != "23" {
		t.Fatal("wrong typed request")
	}
}
