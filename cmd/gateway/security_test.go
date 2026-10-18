package main

import (
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/contract"
)

// TEST-SECURITY-013: REQ-014 AC-4.
func TestConfigManagerPeerIsLimitedToRegisteredTypedActivation(t *testing.T) {
	cfg := &config.Config{Operations: []config.OperationDefinition{{Name: "reload-cats", ExecutorKind: config.ExecutorSystemd}, {Name: "process-op", ExecutorKind: config.ExecutorProcess}, {Name: "parameterized", ExecutorKind: config.ExecutorSystemd, Parameters: []config.ParameterDefinition{{Name: "x", Type: config.ParamTypeString}}}}}
	registry := &configmanager.Registry{Resources: []configmanager.Resource{{Profile: "application", ActivationOperation: "reload-cats"}}}
	allowed, err := validatedManagerOperations(cfg, registry, "application")
	if err != nil {
		t.Fatal(err)
	}
	authorize := configManagerPeerAuthorizer("application", allowed)
	valid := &contract.ExecutionRequest{Command: "reload-cats", Metadata: contract.Metadata{BrokerType: "config-manager"}, Arguments: map[string]string{}}
	if err := authorize(valid); err != nil {
		t.Fatal(err)
	}
	for _, request := range []*contract.ExecutionRequest{{Command: "process-op", Metadata: contract.Metadata{BrokerType: "config-manager"}}, {Command: "reload-cats", Metadata: contract.Metadata{BrokerType: "ssh"}}, {Command: "reload-cats", Metadata: contract.Metadata{BrokerType: "config-manager"}, Arguments: map[string]string{"x": "y"}}} {
		if authorize(request) == nil {
			t.Fatalf("unauthorized request accepted: %+v", request)
		}
	}
	registry.Resources[0].ActivationOperation = "process-op"
	if _, err := validatedManagerOperations(cfg, registry, "application"); err == nil {
		t.Fatal("process activation accepted")
	}
	registry.Resources[0].ActivationOperation = "parameterized"
	if _, err := validatedManagerOperations(cfg, registry, "application"); err == nil {
		t.Fatal("parameterized activation accepted")
	}
	if _, err := validatedManagerOperations(cfg, registry, "unknown"); err == nil {
		t.Fatal("unregistered manager profile accepted")
	}
}

func TestBrokerPeerRequiresMatchingMetadata(t *testing.T) {
	if err := brokerPeerAuthorizer("ssh")(&contract.ExecutionRequest{Metadata: contract.Metadata{BrokerType: "ssh"}}); err != nil {
		t.Fatal(err)
	}
	if brokerPeerAuthorizer("ssh")(&contract.ExecutionRequest{Metadata: contract.Metadata{BrokerType: "config-manager"}}) == nil {
		t.Fatal("non-SSH metadata accepted from Broker UID")
	}
	if err := brokerPeerAuthorizer("console")(&contract.ExecutionRequest{Metadata: contract.Metadata{BrokerType: "console"}}); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayAcceptsEmptyResourceProfilesWithNoActivationGrants(t *testing.T) {
	cfg := &config.Config{Operations: []config.OperationDefinition{{Name: "restart-app", ExecutorKind: config.ExecutorSystemd}}}
	for _, raw := range []string{
		`{"version":"resources-v2","resources":[]}`,
		`{"version":"resources-v2","resources":[{"id":"app","profile":"application","path":"/etc/product/options.ini","format":"ini","access":"read-only","fields":[{"section":"Server","key":"Port","type":"integer"}],"permissions":{"read_roles":["viewer"]}}]}`,
	} {
		registry, err := configmanager.ParseRegistry([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		for _, profile := range []string{"application", "database", "network"} {
			t.Run(profile, func(t *testing.T) {
				allowed, err := validatedManagerOperations(cfg, registry, profile)
				if err != nil {
					t.Fatalf("valid empty activation profile prevented startup: %v", err)
				}
				if len(allowed) != 0 {
					t.Fatalf("unregistered activation grants: %v", allowed)
				}
				authorize := configManagerPeerAuthorizer(profile, allowed)
				for _, request := range []*contract.ExecutionRequest{nil, {Command: "restart-app", Metadata: contract.Metadata{BrokerType: "config-manager"}}, {Command: "restart-app", Metadata: contract.Metadata{BrokerType: "ssh"}}} {
					if authorize(request) == nil {
						t.Fatal("empty profile allowed execution")
					}
				}
			})
		}
		if _, err := validatedManagerOperations(cfg, registry, "unknown"); err == nil {
			t.Fatal("unknown profile accepted")
		}
	}
}
