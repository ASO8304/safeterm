package configmanager

import (
	"context"
	"strings"
	"testing"
)

func TestContextRegistryRejectsAmbiguousOrUnregisteredMappings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context ConfigContext
	}{
		{"empty", ConfigContext{}},
		{"bad path", ConfigContext{Path: []string{"tunnel;id"}, Fields: []FieldAlias{{Name: "value", Section: "cats", Key: "Cat_Port"}}}},
		{"missing field", ConfigContext{Path: []string{"tunnel"}, Fields: []FieldAlias{{Name: "value", Section: "cats", Key: "Unknown"}}}},
		{"bad alias", ConfigContext{Path: []string{"tunnel"}, Fields: []FieldAlias{{Name: "value name", Section: "cats", Key: "Cat_Port"}}}},
		{"duplicate name", ConfigContext{Path: []string{"tunnel"}, Fields: []FieldAlias{{Name: "value", Section: "cats", Key: "Cat_Port"}, {Name: "value", Section: "cats", Key: "Password"}}}},
		{"duplicate coordinate", ConfigContext{Path: []string{"tunnel"}, Fields: []FieldAlias{{Name: "first", Section: "cats", Key: "Cat_Port"}, {Name: "second", Section: "cats", Key: "Cat_Port"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := testRegistry(writeTestINI(t), false)
			registry.Resources[0].Contexts = []ConfigContext{tc.context}
			if err := ValidateRegistry(registry); err == nil {
				t.Fatalf("accepted %+v", tc.context)
			}
		})
	}
	for _, paths := range [][][]string{{{"tunnel"}, {"tunnel"}}, {{"tunnel"}, {"tunnel", "tcp"}}, {{"tunnel", "tcp"}, {"tunnel"}}} {
		items := []ResourceSummary{}
		for _, path := range paths {
			items = append(items, ResourceSummary{ID: "different-id", Contexts: []ConfigContext{{Path: path}}})
		}
		if err := ValidateContextRoutes(items); err == nil {
			t.Fatalf("accepted conflicting paths %v", paths)
		}
	}
	items := []ResourceSummary{{ID: "tunnel"}, {ID: "other", Contexts: []ConfigContext{{Path: []string{"tunnel"}}}}}
	if err := ValidateContextRoutes(items); err == nil {
		t.Fatal("context shadowed legacy resource")
	}
}

func TestContextMetadataIsCopiedAndCannotActivate(t *testing.T) {
	registry := testRegistry(writeTestINI(t), false)
	registry.Resources[0].Contexts = []ConfigContext{{Path: []string{"tunnel"}, Fields: []FieldAlias{{Name: "Value_1", Section: "cats", Key: "Cat_Port"}}}}
	manager, err := NewManager("application", registry, nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	items, err := manager.List(context.Background(), actor("alice", "viewer"))
	if err != nil {
		t.Fatal(err)
	}
	items[0].Contexts[0].Path[0] = "hijacked"
	items[0].Contexts[0].Fields[0].Key = "Password"
	next, err := manager.List(context.Background(), actor("alice", "viewer"))
	if err != nil {
		t.Fatal(err)
	}
	if next[0].Contexts[0].Path[0] != "tunnel" || next[0].Contexts[0].Fields[0].Key != "Cat_Port" {
		t.Fatal("caller mutated manager routing")
	}
	registry.Resources[0].ActivationOperation = "restart_app"
	registry.Resources[0].Permissions.ActivateRoles = []string{"operator"}
	if err := ValidateRegistry(registry); err == nil || !strings.Contains(err.Error(), "save files only") {
		t.Fatalf("logical save may activate: %v", err)
	}
}
