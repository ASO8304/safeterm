package networkmanager

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRouteFieldValidationRejectsInvalidInputWithoutChangingDraft(t *testing.T) {
	view := View{Interfaces: map[string]Interface{"ens37": {Name: "ens37", PersistentName: "ip1"}}, Routes: map[string]Route{"route-office": {Name: "route-office", Interface: "ens37", Destination: "10.0.0.0/8", Gateway: "192.168.222.1"}}}
	for _, tc := range []struct{ field, value, hint string }{
		{"gateway", "192.168.222.1/24", "without a CIDR prefix"},
		{"gateway", "not-an-ip", "unicast IP"},
		{"gateway", "not-an-ip/24", "e.g. set gateway"},
		{"gateway", "0.0.0.0/0", "e.g. set gateway"},
		{"gateway", "::ffff:224.0.0.1", "unicast IP"},
		{"destination", "10.1.2.3/8", "10.0.0.0/8"},
		{"destination", "10.0.0.0", "network CIDR"},
		{"interface", "ip1", "current name"},
		{"interface", "missing", "set interface ?"},
	} {
		_, err := Modify(view, []Change{{Kind: "route", Name: "route-office", Action: "set", Field: tc.field, Values: []string{tc.value}}})
		var issue *RouteValidationError
		if !errors.As(err, &issue) || issue.Route.Name != "route-office" || issue.Field != tc.field || !strings.Contains(issue.Problem, tc.hint) {
			t.Fatal(tc, err)
		}
		if view.Routes["route-office"].Gateway != "192.168.222.1" || view.Routes["route-office"].Destination != "10.0.0.0/8" {
			t.Fatal("rejected field changed baseline")
		}
	}
}

func TestRouteCrossFieldValidationWaitsUntilSave(t *testing.T) {
	view := View{Interfaces: map[string]Interface{"ip0": {Name: "ip0"}}, Routes: map[string]Route{"uplink": {Name: "uplink", Interface: "ip0", Destination: "0.0.0.0/0", Gateway: "192.0.2.1"}}}
	changed, err := Modify(view, []Change{{Kind: "route", Name: "uplink", Action: "set", Field: "destination", Values: []string{"::/0"}}})
	if err != nil {
		t.Fatal("field syntax blocked a staged IP-family transition", err)
	}
	if err := ValidateView(changed); err == nil || !strings.Contains(err.Error(), "same IP address family") {
		t.Fatal("mismatched route could be saved", err)
	}
	changed, err = Modify(changed, []Change{{Kind: "route", Name: "uplink", Action: "set", Field: "gateway", Values: []string{"2001:db8::1"}}})
	if err != nil || ValidateView(changed) != nil || reflect.DeepEqual(view, changed) {
		t.Fatal("valid corrected route rejected", err)
	}
}
