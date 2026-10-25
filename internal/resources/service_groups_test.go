package resources

import (
	"path/filepath"
	"reflect"
	"testing"

	"safeterm/internal/config"
)

func TestResourceServiceGroupCompilesNamedOrderedPlans(t *testing.T) {
	base, err := config.LoadConfig(filepath.Join("..", "defaults", "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	named, err := Parse("akbar-all.json", []byte(`{"id":"akbar-all","description":"Application","service":{"units":["database.service","api.service","web.service"],"timeout_seconds":180,"actions":{"restart":{"roles":["service-operator"]},"status":{"roles":["service-viewer"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _, err := Compile(base, []Named{named})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"restart", "status"} {
		found := false
		for _, op := range cfg.Operations {
			if op.Name != OperationID("akbar-all", "service", action) {
				continue
			}
			found = true
			if op.Systemd.Alias != "akbar-all" || op.Systemd.Unit != "" || op.Systemd.TimeoutSeconds != 180 || !reflect.DeepEqual(op.Systemd.Units, []string{"database.service", "api.service", "web.service"}) {
				t.Fatalf("bad plan %+v", op.Systemd)
			}
			if words, _ := OperationWords(op.Name); !reflect.DeepEqual(words, []string{"service", action, "akbar-all"}) {
				t.Fatalf("unsafe command path %v", words)
			}
		}
		if !found {
			t.Fatal("missing action", action)
		}
	}
	for _, service := range []string{
		`{"unit":"nginx.service","units":["api.service"],"actions":{"restart":{"roles":["service-operator"]}}}`,
		`{"units":["api.service","api.service"],"actions":{"restart":{"roles":["service-operator"]}}}`,
	} {
		item, err := Parse("bad.json", []byte(`{"id":"bad","description":"Bad","service":`+service+`}`))
		if err == nil {
			_, _, _, err = Compile(base, []Named{item})
		}
		if err == nil {
			t.Fatalf("accepted %s", service)
		}
	}
}
