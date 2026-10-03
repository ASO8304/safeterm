package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/contract"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
)

func main() {
	configDir := flag.String("config", "/etc/safeterm", "catalog directory")
	socketPath := flag.String("socket", "/run/safeterm-gateway/gateway.sock", "Gateway socket")
	brokerUser := flag.String("broker-user", "safeterm-broker", "allowed local Broker user")
	consoleUser := flag.String("console-user", "safeterm", "allowed local console Broker user")
	configManagerUsers := flag.String("config-manager-users", "application=safeterm-config-application,database=safeterm-config-database,network=safeterm-config-network", "comma-separated profile=user Config Manager activation clients")
	configRegistry := flag.String("config-resource-registry", "/etc/safeterm/config-manager/resources.json", "root-owned Config Manager resource registry")
	auditSocket := flag.String("audit-socket", "/run/safeterm-audit/audit.sock", "Audit Service socket")
	auditUser := flag.String("audit-user", "safeterm-audit", "trusted Audit Service user")
	catalogPublicKey := flag.String("catalog-public-key", "", "trusted catalog Ed25519 public key (defaults under config directory)")
	catalogSignature := flag.String("catalog-signature", "", "detached catalog signature (defaults under config directory)")
	flag.Parse()
	if err := run(*configDir, *socketPath, *brokerUser, *consoleUser, *configManagerUsers, *configRegistry, *auditSocket, *auditUser, *catalogPublicKey, *catalogSignature); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configDir, socketPath, brokerUser, consoleUser, configManagerUsers, configRegistry, auditSocket, auditUser, catalogPublicKey, catalogSignature string) error {
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return err
	}
	if catalogPublicKey == "" {
		catalogPublicKey = filepath.Join(configDir, "trust", "catalog-ed25519.pub")
	}
	if catalogSignature == "" {
		catalogSignature = filepath.Join(configDir, "catalog.sig")
	}
	current, err := user.Current()
	if err != nil || current.Username != "safeterm-gateway" {
		return fmt.Errorf("gateway: must run as OS user %q", "safeterm-gateway")
	}
	store, err := isolation.NewVerifiedStore(cfg, func(candidate *config.Config) error {
		return integrity.VerifyCatalogFiles(candidate, catalogPublicKey, catalogSignature)
	})
	if err != nil {
		return err
	}
	clients := make(map[config.RunnerProfile]isolation.Client, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		account, err := user.Lookup(profile.RunAsUser)
		if err != nil {
			return fmt.Errorf("gateway: resolve profile %q user: %w", profile.Name, err)
		}
		uid, err := strconv.ParseUint(account.Uid, 10, 32)
		if err != nil {
			return fmt.Errorf("gateway: parse profile %q UID: %w", profile.Name, err)
		}
		client, err := isolation.NewUnixClient(profile.SocketPath, []uint32{uint32(uid)})
		if err != nil {
			return err
		}
		clients[profile.Name] = client
	}
	auditAccount, err := user.Lookup(auditUser)
	if err != nil {
		return fmt.Errorf("gateway: resolve Audit Service user: %w", err)
	}
	auditUID, err := strconv.ParseUint(auditAccount.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("gateway: parse Audit Service UID: %w", err)
	}
	auditClient, err := audit.NewUnixClient(auditSocket, []uint32{uint32(auditUID)})
	if err != nil {
		return err
	}
	gateway, err := isolation.NewGatewayWithSink(store, clients, auditClient)
	if err != nil {
		return err
	}
	brokerAccount, err := user.Lookup(brokerUser)
	if err != nil {
		return fmt.Errorf("gateway: resolve Broker user: %w", err)
	}
	brokerUID, err := strconv.ParseUint(brokerAccount.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("gateway: parse Broker UID: %w", err)
	}
	peerPolicies := map[uint32]isolation.PeerPolicy{uint32(brokerUID): {AllowStreaming: true, Authorize: brokerPeerAuthorizer("ssh")}}
	consoleAccount, err := user.Lookup(consoleUser)
	if err != nil {
		return fmt.Errorf("gateway: resolve console Broker user: %w", err)
	}
	consoleUID, err := strconv.ParseUint(consoleAccount.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("gateway: parse console Broker UID: %w", err)
	}
	if consoleUID == brokerUID {
		return fmt.Errorf("gateway: console and SSH Broker UIDs must be distinct")
	}
	peerPolicies[uint32(consoleUID)] = isolation.PeerPolicy{AllowStreaming: true, Authorize: brokerPeerAuthorizer("console")}
	resourceRegistry, err := configmanager.LoadRegistry(configRegistry)
	if err != nil {
		return fmt.Errorf("gateway: load Config Manager registry: %w", err)
	}
	for _, mapping := range strings.Split(configManagerUsers, ",") {
		parts := strings.Split(strings.TrimSpace(mapping), "=")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("gateway: Config Manager clients must use profile=user syntax")
		}
		profile, name := parts[0], parts[1]
		account, lookupErr := user.Lookup(name)
		if lookupErr != nil {
			return fmt.Errorf("gateway: resolve Config Manager user %q: %w", name, lookupErr)
		}
		value, parseErr := strconv.ParseUint(account.Uid, 10, 32)
		if parseErr != nil {
			return parseErr
		}
		uid := uint32(value)
		if _, exists := peerPolicies[uid]; exists {
			return fmt.Errorf("gateway: IPC peer UIDs must be distinct")
		}
		allowed, policyErr := validatedManagerOperations(cfg, resourceRegistry, profile)
		if policyErr != nil {
			return policyErr
		}
		peerPolicies[uid] = isolation.PeerPolicy{Authorize: configManagerPeerAuthorizer(profile, allowed)}
	}
	server, err := isolation.ListenUnixStreamingWithPeerPolicies(socketPath, 0o660, peerPolicies, gateway, gateway)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}

func brokerPeerAuthorizer(brokerType string) func(*contract.ExecutionRequest) error {
	return func(request *contract.ExecutionRequest) error {
		if request == nil || request.Metadata.BrokerType != brokerType {
			return fmt.Errorf("gateway: Broker peer must use %s identity", brokerType)
		}
		return nil
	}
}

func validatedManagerOperations(cfg *config.Config, registry *configmanager.Registry, profile string) (map[string]bool, error) {
	operations := map[string]config.OperationDefinition{}
	for _, operation := range cfg.Operations {
		operations[operation.Name] = operation
	}
	allowed := map[string]bool{}
	profileFound := false
	for _, resource := range registry.Resources {
		if resource.Profile != profile {
			continue
		}
		profileFound = true
		for _, name := range []string{resource.ActivationOperation, resource.HealthCheckOperation} {
			if name == "" {
				continue
			}
			operation, ok := operations[name]
			if !ok || operation.ExecutorKind != config.ExecutorSystemd || operation.LongRunning || len(operation.Parameters) != 0 {
				return nil, fmt.Errorf("gateway: Config Manager operation %q must be a known bounded argument-free systemd operation", name)
			}
			allowed[name] = true
		}
	}
	if !profileFound {
		return nil, fmt.Errorf("gateway: Config Manager profile %q has no registered resources", profile)
	}
	return allowed, nil
}

func configManagerPeerAuthorizer(profile string, allowed map[string]bool) func(*contract.ExecutionRequest) error {
	return func(request *contract.ExecutionRequest) error {
		if request == nil || request.Metadata.BrokerType != "config-manager" || len(request.Arguments) != 0 || !allowed[request.Command] {
			return fmt.Errorf("gateway: Config Manager profile %q cannot invoke operation", profile)
		}
		return nil
	}
}
