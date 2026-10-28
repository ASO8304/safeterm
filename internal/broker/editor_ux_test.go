package broker

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/networkmanager"
)

func uxNetworkFixture(t *testing.T) (*Shell, *networkFixtureAPI, *bytes.Buffer) {
	t.Helper()
	s, _, out := editorFixture(t)
	api := &networkFixtureAPI{view: networkmanager.View{Version: "base", States: map[string]string{"ip0": "up", "ip1": "down"}, Interfaces: map[string]networkmanager.Interface{
		"ip0": {Name: "ip0", Addresses: []string{"192.168.222.10/24"}, ReadOnly: "interface is outside administrator policy"},
		"ip1": {Name: "ip1", Files: []string{"etc/netplan/01-netcfg.yaml"}},
	}, Routes: map[string]networkmanager.Route{}}}
	s.SetNetworkClient(api)
	return s, api, out
}

func TestEditorDirtyPromptsReflectEffectiveValues(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "config network interface", "edit ip1", "set dhcp4 enable")
	if s.prompt() != "safeterm::alice(network/interface/ip1)*> " {
		t.Fatal(s.prompt())
	}
	editorCommands(t, s, "next")
	if s.prompt() != "safeterm::alice(network/interface)*> " {
		t.Fatal(s.prompt())
	}
	editorCommands(t, s, "edit ip1", "set dhcp4 disable", "unset addresses")
	if strings.Contains(s.prompt(), "*") {
		t.Fatal("restoring the base value remains dirty")
	}
	out.Reset()
	editorCommands(t, s, "diff", "back")
	if !strings.Contains(out.String(), "No changes.") || strings.Contains(out.String(), "Discarded unsaved") {
		t.Fatal(out.String())
	}
	if len(api.requests) != 1 {
		t.Fatal("local editing called the manager")
	}

	s, _, _ = editorFixture(t)
	editorCommands(t, s, "config options", "edit Server", "set Port 9090")
	if s.prompt() != "safeterm::alice(options/Server)*> " {
		t.Fatal(s.prompt())
	}
	editorCommands(t, s, "set Port 8080")
	if strings.Contains(s.prompt(), "*") || len(s.editing.changes) != 0 {
		t.Fatal("restored application value remains dirty")
	}
}

func TestGlobalsInsideAllEditorsDoNotSaveAndHelpCannotExit(t *testing.T) {
	for _, kind := range []string{"application", "logical", "interface", "route"} {
		for _, dirty := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: "/dirty", false: "/clean"}[dirty], func(t *testing.T) {
				var s *Shell
				var out *bytes.Buffer
				var api *networkFixtureAPI
				var path string
				var before []byte
				switch kind {
				case "application":
					s, path, out = editorFixture(t)
					before, _ = os.ReadFile(path)
					editorCommands(t, s, "config options", "edit Server")
					if dirty {
						editorCommands(t, s, "set Port 9090")
					}
				case "logical":
					s, path, out = logicalConfigFixture(t)
					before, _ = os.ReadFile(path)
					editorCommands(t, s, "config tunnel")
					if dirty {
						editorCommands(t, s, "set Value_1 1400")
					}
				default:
					s, api, out = uxNetworkFixture(t)
					if kind == "interface" {
						editorCommands(t, s, "config network interface", "edit ip1")
						if dirty {
							editorCommands(t, s, "set mtu 1400")
						}
					} else {
						editorCommands(t, s, "config network route", "edit office")
						if dirty {
							editorCommands(t, s, "set interface ip1")
						}
					}
				}
				prompt := s.prompt()
				out.Reset()
				editorCommands(t, s, "whoami", "clear", "exit ?", "whoami ?", "clear ?")
				if !strings.Contains(out.String(), "user: alice") || !strings.Contains(out.String(), "\x1b[2J\x1b[H") || s.prompt() != prompt {
					t.Fatal("global command changed the draft", out)
				}
				exit, err := s.executeLine(context.Background(), "exit", nil)
				if !exit || err != nil || s.editing != nil || s.networkEdit != nil {
					t.Fatal(exit, err)
				}
				if strings.Contains(out.String(), "Unsaved drafts were discarded") != dirty {
					t.Fatal(out.String())
				}
				if path != "" {
					after, _ := os.ReadFile(path)
					if !bytes.Equal(before, after) {
						t.Fatal("exit saved a file")
					}
				}
				if api != nil && len(api.requests) != 1 {
					t.Fatal("global command called manager")
				}
			})
		}
	}
}

func TestNetworkTextDiffAndDetailShowStagingWithoutRPC(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "config network interface", "edit ip1", "set dhcp4 enable", "append addresses 10.0.0.5/24")
	out.Reset()
	editorCommands(t, s, "diff")
	want := "Interface: ip1\r\n  DHCP4:\r\n-   off\r\n+   on\r\n  Addresses:\r\n+   10.0.0.5/24\r\n"
	if out.String() != want {
		t.Fatalf("diff:\n%s", out)
	}
	out.Reset()
	editorCommands(t, s, "show")
	for _, want := range []string{"Interface: ip1 (Editing Allowed)", "* Unsaved edits staged", "DHCP4:          on (staged)", "Addresses:      10.0.0.5/24 (staged)", "DNS Servers:    System default", "Search Domains: System default", "/etc/netplan/01-netcfg.yaml"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	editorCommands(t, s, "next")
	out.Reset()
	editorCommands(t, s, "show")
	for _, want := range []string{"NAME", "STATE", "DHCP4", "ADDRESSES", "EDITING", "ip1 *", "up", "down", "Read-only (Policy restricted)", "Allowed"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if len(api.requests) != 1 {
		t.Fatal("show/diff made a request")
	}
}

func TestMissingCIDRRejectedImmediatelyWithoutChangingDraft(t *testing.T) {
	for _, command := range []string{"set addresses 10.0.0.5", "append addresses 2001:db8::1", "set addresses 10.0.0.5/24 10.0.0.6"} {
		s, api, _ := uxNetworkFixture(t)
		editorCommands(t, s, "config network interface", "edit ip1")
		before := s.networkEdit.view
		_, err := s.executeLine(context.Background(), command, nil)
		if err == nil || !strings.Contains(err.Error(), "is missing a CIDR prefix. Format as Address/Prefix (e.g., 10.50.0.0/16).") {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s.networkEdit.view, before) || s.editorDirty() || len(api.requests) != 1 {
			t.Fatal("invalid address mutated draft/backend")
		}
	}
}

func TestTCPTestCanonicalNameAndDeprecatedAliasKeepAuthorization(t *testing.T) {
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
	if got := s.completionCandidates(context.Background(), "execute tcp", "tcp"); !reflect.DeepEqual(got, []string{"tcp-test"}) {
		t.Fatal(got)
	}
	editorCommands(t, s, "execute ?", "execute tcp-test ?", "execute telnet ?")
	if strings.Contains(out.String(), "Use 'execute tcp-test'") || len(client.requests) != 0 {
		t.Fatal("help ran alias")
	}
	for _, name := range []string{"tcp-test", "telnet"} {
		out.Reset()
		input := make(chan byte, 1)
		input <- 0x03
		exit, err := s.executeLine(context.Background(), "execute "+name+" 127.0.0.1 443", input)
		if err != nil || exit {
			t.Fatal(exit, err)
		}
		request := client.requests[len(client.requests)-1]
		if request.Command != "telnet" || request.Arguments["port"] != "443" {
			t.Fatal(request)
		}
		if strings.Contains(out.String(), "Use 'execute tcp-test' for port probing") != (name == "telnet") {
			t.Fatal(out.String())
		}
	}
	viewer, _ := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader(""), out)
	for _, name := range []string{"tcp-test", "telnet"} {
		if _, err := viewer.executeLine(context.Background(), "execute "+name+" 127.0.0.1 443", nil); err == nil {
			t.Fatal("unauthorized TCP probe")
		}
	}
}

func TestLogicalNextStagesThenResumesAndBackDiscards(t *testing.T) {
	s, path, out := logicalConfigFixture(t)
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config application cat", "set enabled false", "next")
	if s.editing != nil || !reflect.DeepEqual(s.configMenu, []string{"application"}) || !s.editorDirty() {
		t.Fatal("next did not move up with draft")
	}
	editorCommands(t, s, "cat", "show")
	if !strings.Contains(out.String(), "Resumed unsaved edits.") || s.editing.changes[0].Value != "false" {
		t.Fatal("draft not resumed")
	}
	editorCommands(t, s, "next", "back")
	if len(s.stagedLogical) != 0 || !strings.Contains(out.String(), "Discarded unsaved edits for 'cat'.") {
		t.Fatal("back kept draft")
	}
	editorCommands(t, s, "config application cat")
	if len(s.editing.changes) != 0 {
		t.Fatal("discarded draft returned")
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(before, current) {
		t.Fatal("next or back wrote a file")
	}
}

func TestRouteDiffShowsCreationDeletionAndRestoredDraft(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	api.view.Routes["office"] = networkmanager.Route{Name: "office", Interface: "ip1", Destination: "10.0.0.0/8", Gateway: "192.0.2.1"}
	editorCommands(t, s, "config network route", "delete office", "edit default", "set interface ip1", "set destination 0.0.0.0/0", "set gateway 192.0.2.2")
	out.Reset()
	editorCommands(t, s, "diff")
	for _, want := range []string{"Route: default", "+   ip1", "+   0.0.0.0/0", "+   192.0.2.2", "Route: office", "-   ip1", "-   10.0.0.0/8", "-   192.0.2.1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	editorCommands(t, s, "back", "edit office", "set interface ip1", "set destination 10.0.0.0/8", "set gateway 192.0.2.1")
	if s.editorDirty() {
		t.Fatal("restored route remains dirty")
	}
	if len(api.requests) != 1 {
		t.Fatal("route diff called backend")
	}
}

func TestTextDiffPreservesListOrderingAndPrefixesMultilineValues(t *testing.T) {
	s, _, out := editorFixture(t)
	s.diffValues([]string{"a", "b"}, []string{"b", "a"})
	if out.String() != "-   a\r\n    b\r\n+   a\r\n" {
		t.Fatal(out.String())
	}
	out.Reset()
	s.diffValues([]string{"old\nvalue"}, []string{"new\nvalue"})
	if out.String() != "-   old\r\n-   value\r\n+   new\r\n+   value\r\n" {
		t.Fatal(out.String())
	}
}

func TestMissingCIDRHasReadableErrorInInteractiveShell(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	s.input = strings.NewReader("config network interface\redit ip1\rappend addresses 192.168.222.100\rexit\r")
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Error: '192.168.222.100' is missing a CIDR prefix. Format as Address/Prefix (e.g., 10.50.0.0/16).") || strings.Contains(out.String(), "Unsaved drafts were discarded") || len(api.requests) != 1 {
		t.Fatal(out.String())
	}
}

func TestCanonicalEditorHelpAndCompletion(t *testing.T) {
	for _, network := range []bool{false, true} {
		s, _, out := uxNetworkFixture(t)
		if network {
			editorCommands(t, s, "config network interface", "edit ip1")
		} else {
			editorCommands(t, s, "config options", "edit Server")
		}
		out.Reset()
		editorCommands(t, s, "?", "save ?", "cancel ?", "back ?", "next ?")
		for _, want := range []string{"save", "cancel", "clear", "whoami", "exit", backDescription, nextDescription} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("missing %q: %s", want, out)
			}
		}
		commands := s.completionCandidates(context.Background(), "", "")
		for _, old := range []string{"end", "abort"} {
			for _, command := range commands {
				if command == old {
					t.Fatal("legacy command in completion", commands)
				}
			}
			if _, err := s.executeLine(context.Background(), old, nil); err == nil {
				t.Fatal("legacy editor command accepted", old)
			}
		}
	}
}

func TestExitDiscardsRetainedLogicalDraftEvenWithCleanActiveEditor(t *testing.T) {
	s, path, out := logicalConfigFixture(t)
	before, _ := os.ReadFile(path)
	editorCommands(t, s, "config tunnel", "set Value_1 1400", "next", "application", "cat")
	out.Reset()
	exit, err := s.executeLine(context.Background(), "exit", nil)
	if err != nil || !exit || len(s.stagedLogical) != 0 || !strings.Contains(out.String(), "Unsaved drafts were discarded") {
		t.Fatal(exit, err, out)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("exit wrote retained draft")
	}
}
