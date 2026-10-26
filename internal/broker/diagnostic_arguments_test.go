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
