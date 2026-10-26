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

	// Fresh defaults expose no service branch, even to service operators.
	defaultCatalog, err := NewCatalog(base)
	if err != nil {
		t.Fatal(err)
	}
	defaultClient := &fakeGateway{}
	defaultShell, err := NewShell(defaultClient, defaultCatalog, Principal{UserID: "admin", Roles: []string{"viewer", "service-viewer", "service-operator"}}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range defaultShell.completionCandidates(context.Background(), "execute ", "") {
		if choice == "service" {
			t.Fatal("fresh catalog exposes service branch")
		}
	}
	if got := defaultShell.completionCandidates(context.Background(), "execute service status ", ""); len(got) != 0 {
		t.Fatalf("implicit service targets: %v", got)
	}
	if _, err := defaultShell.executeLine(context.Background(), "execute service status gateway", nil); err == nil {
		t.Fatal("removed packaged service target still accepted")
	}
	if len(defaultClient.requests) != 0 {
		t.Fatal("default service command dispatched")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "runtime", "resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = configmanager.ParseRegistry(data)
	if err != nil {
		t.Fatal(err)
	}
	item, err := resources.Parse("product.json", []byte(`{"id":"product","description":"Product","service":{"unit":"product.service","actions":{"status":{"roles":["service-viewer"]},"restart":{"roles":["service-operator"]},"start":{"roles":["service-operator"]},"stop":{"roles":["service-operator"]},"enable":{"roles":["service-operator"]},"disable":{"roles":["service-operator"]}}},"logs":[{"id":"main","path":"/var/log/product/main.log","roles":["log-viewer"]}]}`))
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
	if got := s.completionCandidates(context.Background(), "execute service status ", ""); strings.Join(got, ",") != "product" {
		t.Fatalf("only registered service should be listed: %v", got)
	}
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

	// All six actions for an external system service retain exact signed IDs
	// and per-action authorization, including terminal completion.
	viewerChoices := s.completionCandidates(context.Background(), "execute service ", "")
	if strings.Join(viewerChoices, ",") != "status" {
		t.Fatalf("viewer actions=%v", viewerChoices)
	}
	operatorClient := &fakeGateway{}
	operator, err := NewShell(operatorClient, catalog, Principal{UserID: "operator", Roles: []string{"service-viewer", "service-operator"}}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(operator.completionCandidates(context.Background(), "execute service ", ""), ","); got != "disable,enable,restart,start,status,stop" {
		t.Fatalf("operator actions=%s", got)
	}
	for _, action := range []string{"status", "restart", "start", "stop", "enable", "disable"} {
		if got := operator.completionCandidates(context.Background(), "execute service "+action+" pro", "pro"); strings.Join(got, ",") != "product" {
			t.Fatalf("%s completion=%v", action, got)
		}
		if _, err := operator.executeLine(context.Background(), "execute service "+action+" product", nil); err != nil {
			t.Fatal(err)
		}
		last := operatorClient.requests[len(operatorClient.requests)-1]
		if last.Command != resources.OperationID("product", "service", action) || len(last.Arguments) != 0 {
			t.Fatalf("wrong service request: %+v", last)
		}
	}
	if _, err := operator.executeLine(context.Background(), "execute service restart unregistered", nil); err == nil {
		t.Fatal("unregistered service dispatched")
	}
	s.principal.Roles = []string{"log-viewer"}
	if got := strings.Join(s.completionCandidates(context.Background(), "execute logs ", ""), ","); got != "product" {
		t.Fatalf("only registered resources should appear in logs menu: %q", got)
	}
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
