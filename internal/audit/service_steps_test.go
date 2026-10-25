package audit

import (
	"testing"
	"time"
)

func TestServiceStepMetadataIsRestrictedToValidRunnerEvents(t *testing.T) {
	valid := Event{SchemaVersion: SchemaVersion, RequestID: "request", CatalogVersion: "version", Operation: "group", Component: ComponentRunner, Profile: "service-manager", Outcome: "service-step-started", OccurredAt: time.Now().UTC(), ServiceUnit: "nginx.service", ServiceAction: "start", ServiceStep: 1}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Event){
		func(e *Event) { e.ServiceUnit = "*.service" },
		func(e *Event) { e.ServiceUnit = "../nginx.service" },
		func(e *Event) { e.ServiceAction = "daemon-reload" },
		func(e *Event) { e.ServiceStep = 0 },
		func(e *Event) { e.ServiceStep = 65 },
		func(e *Event) { e.Component = ComponentGateway },
		func(e *Event) { e.Outcome = "accepted" },
	} {
		e := valid
		mutate(&e)
		if err := e.Validate(); err == nil {
			t.Fatalf("accepted invalid step %+v", e)
		}
	}
}
