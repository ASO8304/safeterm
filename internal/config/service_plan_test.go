package config

import "testing"

func TestServicePlanRejectsAmbiguousAndUnsafeTargets(t *testing.T) {
	for _, s := range []SystemdOperationDefinition{
		{Action: "start"},
		{Unit: "one.service", Units: []string{"two.service"}, Action: "start"},
		{Units: []string{"one.service", "one.service"}, Action: "restart"},
		{Units: []string{"*.service"}, Action: "start"},
		{Units: []string{"../../one.service"}, Action: "start"},
		{Units: []string{"one.service;id"}, Action: "start"},
		{Unit: "one.service", Action: "daemon-reload"},
		{Unit: "one.service", Action: "start", Alias: "bad\nname"},
		{Unit: "one.service", Action: "start", TimeoutSeconds: 241},
		{Unit: "one.service", Action: "start", TimeoutSeconds: -1},
	} {
		if err := ValidateSystemdDefinition(&s); err == nil {
			t.Fatalf("accepted invalid plan %+v", s)
		}
	}
	s := SystemdOperationDefinition{Units: make([]string, 33), Action: "start"}
	if err := ValidateSystemdDefinition(&s); err == nil {
		t.Fatal("accepted unbounded group")
	}
}
