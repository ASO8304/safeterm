package broker

import (
	"context"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func TestSaveIdentifiesInvalidDefaultFromInterfaceAndRouteEditors(t *testing.T) {
	for _, kind := range []string{"interface", "route"} {
		t.Run(kind, func(t *testing.T) {
			s, api, out := uxNetworkFixture(t)
			name := "route-4db1e03c2b1a"
			api.view.Routes[name] = networkmanager.Route{Name: name, Interface: "ip1", Destination: "0.0.0.0/0", Gateway: "192.168.222.1/24"}
			editorCommands(t, s, "config network "+kind)
			if kind == "interface" {
				editorCommands(t, s, "edit ip1", "set mtu 1400")
			} else {
				editorCommands(t, s, "add", "set destination 10.0.0.0/8", "set gateway 192.168.222.1", "set interface ip1")
			}
			requests := len(api.requests)
			_, err := s.executeLine(context.Background(), "save", nil)
			if err == nil || !strings.Contains(err.Error(), "Route 1 (Default IPv4) on interface ip1") || !strings.Contains(err.Error(), "set gateway 192.168.222.1") || !strings.Contains(err.Error(), "validated together") || strings.Contains(err.Error(), name) || len(api.requests) != requests || !s.editorDirty() {
				t.Fatal("save obscured invalid imported route or lost draft", err, api.requests)
			}
			if kind == "route" {
				out.Reset()
				editorCommands(t, s, "next", "edit 1", "show")
				if !strings.Contains(out.String(), "Invalid configuration:") || strings.Contains(out.String(), name) {
					t.Fatal("show hid invalid configuration", out.String())
				}
				editorCommands(t, s, "set gateway 192.168.222.1", "save")
				if api.view.Routes[name].Gateway != "192.168.222.1" || len(api.view.Routes) != 2 {
					t.Fatal("repair lost staged route", api.view)
				}
			}
		})
	}
}

func TestInvalidGatewayIsRejectedImmediatelyAndKeepsPreviousValue(t *testing.T) {
	s, api, _ := uxNetworkFixture(t)
	editorCommands(t, s, "config network route", "add", "set destination 10.0.0.0/8", "set gateway 192.168.222.1")
	name := s.networkEdit.item
	before := len(s.networkEdit.changes)
	_, err := s.executeLine(context.Background(), "set gateway 192.168.222.1/24", nil)
	if err == nil || !strings.Contains(err.Error(), "Route 1") || !strings.Contains(err.Error(), "without a CIDR prefix") || s.networkEdit.view.Routes[name].Gateway != "192.168.222.1" || len(s.networkEdit.changes) != before || len(api.requests) != 1 {
		t.Fatal("invalid gateway staged or leaked internal identity", err)
	}
}
