package audit

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestServiceStepIdentityIsBoundedAndUnambiguous(t *testing.T) {
	operation, err := ServiceStepOperation("group", 1, "start", "nginx.service")
	if err != nil {
		t.Fatal(err)
	}
	valid := Event{SchemaVersion: SchemaVersion, RequestID: "request", CatalogVersion: "version", Operation: operation, Component: ComponentRunner, Profile: "service-manager", Outcome: "accepted", OccurredAt: time.Now().UTC()}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	parsed, ok := ParseServiceStep(operation)
	if !ok || parsed.ParentOperation != "group" || parsed.Unit != "nginx.service" || parsed.Index != 1 || parsed.Action != "start" {
		t.Fatal(parsed)
	}
	for _, name := range []string{"service-step:group:0:start:nginx.service", "service-step:group:65:start:nginx.service", "service-step:group:01:start:nginx.service", "service-step:group:1:daemon-reload:nginx.service", "service-step:group:1:start:*.service", "service-step:group:1:start:../nginx.service", "service-step:group:1:start:nginx.service:extra"} {
		e := valid
		e.Operation = name
		if err := e.Validate(); err == nil {
			t.Fatalf("accepted invalid step %s", name)
		}
	}
	e := valid
	e.Component = ComponentGateway
	if err := e.Validate(); err == nil {
		t.Fatal("gateway forged Runner step")
	}
	e = valid
	e.Outcome = "service-step-started"
	if err := e.Validate(); err == nil {
		t.Fatal("unknown V1 outcome accepted")
	}
	// Keep on-disk V1 serialization readable by earlier package binaries.
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"catalog_version", "component", "occurred_at", "operation", "outcome", "profile", "request_id", "schema_version"}) {
		t.Fatalf("service step changed V1 envelope fields: %v", names)
	}
}
