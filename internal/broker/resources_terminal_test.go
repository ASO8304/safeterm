package broker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/resources"
)

func TestGeneratedResourceCommandsUseSignedIDsAndPerActionRoles(t *testing.T) {
	base, err := config.LoadConfig(filepath.Join("..", "defaults", "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "runtime", "resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = configmanager.ParseRegistry(data)
	if err != nil {
		t.Fatal(err)
	}
	item, err := resources.Parse("product.json", []byte(`{"id":"product","description":"Product","service":{"unit":"product.service","actions":{"status":{"roles":["service-viewer"]},"restart":{"roles":["service-operator"]}}},"logs":[{"id":"main","path":"/var/log/product/main.log","roles":["log-viewer"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _, err := resources.Compile(base, []resources.Named{item})
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateOperationPaths(cfg); err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGateway{}
	out := &bytes.Buffer{}
	s, _ := NewShell(client, catalog, Principal{UserID: "viewer", Roles: []string{"service-viewer"}}, strings.NewReader(""), out)
	editorCommands(t, s, "execute service status product")
	if client.requests[0].Command != resources.OperationID("product", "service", "status") {
		t.Fatal("friendly path changed signed ID")
	}
	if _, err = s.executeLine(context.Background(), "execute service restart product", nil); err == nil {
		t.Fatal("viewer restarted service")
	}
	if _, err = s.executeLine(context.Background(), "execute logs product main", nil); err == nil {
		t.Fatal("viewer read log")
	}
	if len(client.requests) != 1 {
		t.Fatal("unauthorized command dispatched")
	}
	s.principal.Roles = []string{"log-viewer"}
	if got := s.completionCandidates(context.Background(), "execute logs product m", "m"); strings.Join(got, ",") != "main" {
		t.Fatalf("completion=%v", got)
	}
	input := make(chan byte, 1)
	input <- 0x03
	if _, err = s.executeLine(context.Background(), "execute logs product main", input); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 || client.requests[1].Command != resources.OperationID("product", "log", "main") || len(client.requests[1].Arguments) != 0 {
		t.Fatal("log accepted caller path or wrong signed ID")
	}
}
