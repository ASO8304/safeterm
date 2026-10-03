//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/term"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
)

const (
	configDir      = "/etc/safeterm"
	identitiesPath = "/etc/safeterm/console/users.json"
	gatewaySocket  = "/run/safeterm-gateway/gateway.sock"
)

var managerEndpoints = []struct{ socket, user string }{
	{"/run/safeterm-config-application/manager.sock", "safeterm-config-application"},
	{"/run/safeterm-config-database/manager.sock", "safeterm-config-database"},
	{"/run/safeterm-config-network/manager.sock", "safeterm-config-network"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("console: an interactive terminal is required")
	}
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("console: resolve OS identity: %w", err)
	}
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return err
	}
	if err := integrity.VerifyCatalogFiles(cfg, filepath.Join(configDir, "trust", "catalog-ed25519.pub"), filepath.Join(configDir, "catalog.sig")); err != nil {
		return err
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		return err
	}
	principal, err := broker.LoadConsolePrincipal(identitiesPath, current.Username, catalog.KnownRoles())
	if err != nil {
		return err
	}
	gatewayUID, err := lookupUID("safeterm-gateway")
	if err != nil {
		return err
	}
	client, err := isolation.NewUnixClient(gatewaySocket, []uint32{gatewayUID})
	if err != nil {
		return err
	}
	services := make([]configmanager.Service, 0, len(managerEndpoints))
	for _, endpoint := range managerEndpoints {
		uid, lookupErr := lookupUID(endpoint.user)
		if lookupErr != nil {
			return lookupErr
		}
		manager, clientErr := configmanager.NewUnixClient(endpoint.socket, []uint32{uid})
		if clientErr != nil {
			return clientErr
		}
		services = append(services, manager)
	}
	configs, err := broker.NewConfigBroker(services)
	if err != nil {
		return err
	}
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("console: enter raw terminal mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), state)
	shell, err := broker.NewShellForBroker(client, catalog, principal, os.Stdin, os.Stdout, configs, "console")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return shell.Run(ctx)
}

func lookupUID(name string) (uint32, error) {
	account, err := user.Lookup(name)
	if err != nil {
		return 0, fmt.Errorf("console: resolve service user %q: %w", name, err)
	}
	value, err := strconv.ParseUint(account.Uid, 10, 32)
	return uint32(value), err
}
