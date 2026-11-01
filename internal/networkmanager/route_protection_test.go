package networkmanager

import (
	"reflect"
	"strings"
	"testing"
)

func TestModifyProtectsIPv4AndIPv6DefaultRoutes(t *testing.T) {
	for _, destination := range []string{"0.0.0.0/0", "::/0"} {
		t.Run(destination, func(t *testing.T) {
			view := View{Routes: map[string]Route{"uplink": {Name: "uplink", Destination: destination}}}
			for _, changes := range [][]Change{
				{{Kind: "route", Name: "uplink", Action: "delete"}},
				{{Kind: "route", Name: "uplink", Action: "set", Field: "destination", Values: []string{"10.0.0.0/8"}}, {Kind: "route", Name: "uplink", Action: "delete"}},
			} {
				if _, err := Modify(view, changes); err == nil || !strings.Contains(err.Error(), "default routes cannot be deleted") {
					t.Fatal("default deletion accepted", changes, err)
				}
				if view.Routes["uplink"].Destination != destination {
					t.Fatal("rejected changes mutated the original")
				}
			}
		})
	}
}

func TestDefaultProtectionAllowsGatewayEditsAndOtherRouteDeletion(t *testing.T) {
	view := View{Routes: map[string]Route{
		"uplink": {Name: "uplink", Destination: "0.0.0.0/0", Gateway: "192.0.2.1"},
		"office": {Name: "office", Destination: "10.0.0.0/8"},
	}}
	changed, err := Modify(view, []Change{{Kind: "route", Name: "uplink", Action: "set", Field: "gateway", Values: []string{"192.0.2.254"}}, {Kind: "route", Name: "office", Action: "delete"}})
	if err != nil || changed.Routes["uplink"].Gateway != "192.0.2.254" || len(changed.Routes) != 1 {
		t.Fatal("valid route changes rejected", changed, err)
	}
	if err = validateDefaultRouteDeletions(view, changed); err != nil || reflect.DeepEqual(view, changed) {
		t.Fatal("default protection rejected valid changes", err)
	}
}
