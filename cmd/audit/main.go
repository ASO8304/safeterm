package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"

	"safeterm/internal/audit"
)

func main() {
	socketPath := flag.String("socket", "/run/safeterm-audit/audit.sock", "Audit Service socket")
	storePath := flag.String("store", "/var/lib/safeterm/audit.jsonl", "append-only audit store")
	gatewayUser := flag.String("gateway-user", "safeterm-gateway", "Gateway event producer user")
	diagnosticsUser := flag.String("diagnostics-user", "safeterm-diagnostics", "diagnostics Runner user")
	networkUser := flag.String("network-user", "safeterm-network", "network-observer Runner user")
	serviceUser := flag.String("service-user", "safeterm-service-manager", "service-manager Runner user")
	applicationConfigUser := flag.String("application-config-user", "safeterm-config-application", "application Config Manager user")
	databaseConfigUser := flag.String("database-config-user", "safeterm-config-database", "database Config Manager user")
	networkConfigUser := flag.String("network-config-user", "safeterm-config-network", "network Config Manager user")
	flag.Parse()
	if err := run(*socketPath, *storePath, []producerUser{
		{name: *gatewayUser, policy: audit.Producer{Component: audit.ComponentGateway}},
		{name: *diagnosticsUser, policy: audit.Producer{Component: audit.ComponentRunner, Profile: "diagnostics"}},
		{name: *networkUser, policy: audit.Producer{Component: audit.ComponentRunner, Profile: "network-observer"}},
		{name: *serviceUser, policy: audit.Producer{Component: audit.ComponentRunner, Profile: "service-manager"}},
		{name: *applicationConfigUser, policy: audit.Producer{Component: audit.ComponentConfigManager, Profile: "application"}},
		{name: *databaseConfigUser, policy: audit.Producer{Component: audit.ComponentConfigManager, Profile: "database"}},
		{name: *networkConfigUser, policy: audit.Producer{Component: audit.ComponentConfigManager, Profile: "network"}},
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type producerUser struct {
	name   string
	policy audit.Producer
}

func run(socketPath, storePath string, producerUsers []producerUser) error {
	current, err := user.Current()
	if err != nil || current.Username != "safeterm-audit" {
		return fmt.Errorf("audit: must run as OS user %q", "safeterm-audit")
	}
	producers := make(map[uint32][]audit.Producer, len(producerUsers))
	for _, producer := range producerUsers {
		if producer.name == "" {
			return fmt.Errorf("audit: producer user must not be empty")
		}
		account, err := user.Lookup(producer.name)
		if err != nil {
			return fmt.Errorf("audit: resolve allowed user %q: %w", producer.name, err)
		}
		uid, err := strconv.ParseUint(account.Uid, 10, 32)
		if err != nil {
			return fmt.Errorf("audit: parse allowed user %q UID: %w", producer.name, err)
		}
		if _, exists := producers[uint32(uid)]; exists {
			return fmt.Errorf("audit: producer users must have distinct UIDs")
		}
		producers[uint32(uid)] = []audit.Producer{producer.policy}
	}
	store, err := audit.OpenFileStore(storePath)
	if err != nil {
		return err
	}
	defer store.Close()
	server, err := audit.ListenUnix(socketPath, 0o660, producers, store)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}
