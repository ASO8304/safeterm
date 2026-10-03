package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/configmanager"
	"safeterm/internal/isolation"
)

func main() {
	profile := flag.String("profile", "", "resource profile")
	registryPath := flag.String("registry", "/etc/safeterm/config-manager/resources.json", "trusted resource registry")
	socket := flag.String("socket", "", "Unix socket")
	brokerUsers := flag.String("broker-users", "safeterm-broker,safeterm", "comma-separated trusted Broker OS users")
	auditSocket := flag.String("audit-socket", "/run/safeterm-audit/audit.sock", "Audit Service socket")
	auditUser := flag.String("audit-user", "safeterm-audit", "trusted Audit Service OS user")
	gatewaySocket := flag.String("gateway-socket", "/run/safeterm-gateway/gateway.sock", "Node Gateway socket for typed activation")
	gatewayUser := flag.String("gateway-user", "safeterm-gateway", "trusted Node Gateway user")
	runAsUser := flag.String("run-as-user", "", "required operating-system user")
	ttl := flag.Duration("proposal-ttl", 5*time.Minute, "proposal lifetime")
	flag.Parse()
	if err := run(*profile, *registryPath, *socket, *brokerUsers, *auditSocket, *auditUser, *gatewaySocket, *gatewayUser, *runAsUser, *ttl); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(profile, registryPath, socket, brokerUsers, auditSocket, auditUser, gatewaySocket, gatewayUser, runAsUser string, ttl time.Duration) error {
	if profile == "" || socket == "" || runAsUser == "" {
		return fmt.Errorf("config-manager: profile, socket, and run-as-user are required")
	}
	current, err := user.Current()
	if err != nil || current.Username != runAsUser {
		return fmt.Errorf("config-manager: must run as OS user %q", runAsUser)
	}
	registry, err := configmanager.LoadRegistry(registryPath)
	if err != nil {
		return err
	}
	allowedUIDs := []uint32{}
	seenUIDs := map[uint32]bool{}
	for _, name := range strings.Split(brokerUsers, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("config-manager: Broker user names must not be empty")
		}
		uid, lookupErr := lookupUID(name)
		if lookupErr != nil {
			return fmt.Errorf("config-manager: broker identity %q: %w", name, lookupErr)
		}
		if seenUIDs[uid] {
			return fmt.Errorf("config-manager: Broker UIDs must be distinct")
		}
		seenUIDs[uid] = true
		allowedUIDs = append(allowedUIDs, uid)
	}
	auditUID, err := lookupUID(auditUser)
	if err != nil {
		return fmt.Errorf("config-manager: audit identity: %w", err)
	}
	auditClient, err := audit.NewUnixClient(auditSocket, []uint32{auditUID})
	if err != nil {
		return err
	}
	var activator configmanager.TypedActivator
	for _, resource := range registry.Resources {
		if resource.Profile == profile && resource.ActivationOperation != "" {
			gatewayUID, lookupErr := lookupUID(gatewayUser)
			if lookupErr != nil {
				return fmt.Errorf("config-manager: gateway identity: %w", lookupErr)
			}
			gatewayClient, clientErr := isolation.NewUnixClient(gatewaySocket, []uint32{gatewayUID})
			if clientErr != nil {
				return clientErr
			}
			activator, clientErr = configmanager.NewGatewayActivator(gatewayClient)
			if clientErr != nil {
				return clientErr
			}
			break
		}
	}
	manager, err := configmanager.NewManager(profile, registry, nil, activator, auditClient, configmanager.ManagerOptions{ProposalTTL: ttl})
	if err != nil {
		return err
	}
	server, err := configmanager.ListenUnix(socket, 0o660, allowedUIDs, manager)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}

func lookupUID(name string) (uint32, error) {
	account, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(account.Uid, 10, 32)
	return uint32(value), err
}
