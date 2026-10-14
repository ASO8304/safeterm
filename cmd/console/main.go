//go:build linux

package main

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"safeterm/internal/identity"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/term"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
)

const (
	configDir     = "/etc/safeterm"
	gatewaySocket = "/run/safeterm-gateway/gateway.sock"
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
	if err != nil || current.Username != "safeterm" {
		return fmt.Errorf("console: use the shared safeterm OS account")
	}
	// Prevent sibling processes of the shared UID from reading login credentials
	// or tokens through ptrace/proc; prohibit gaining privileges through exec.
	if err = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return err
	}
	if err = unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
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
	identityUID, err := lookupUID(identity.UserName)
	if err != nil {
		return err
	}
	identityClient := identity.NewClient(identity.SocketPath, identityUID)
	shell, err := broker.NewAuthenticatedConsoleShell(client, catalog, os.Stdin, os.Stdout, configs,
		func(ctx context.Context, user string, password []byte) (broker.Principal, time.Time, error) {
			session, err := identityClient.Login(ctx, user, password)
			return broker.Principal{UserID: session.UserID, Roles: session.Roles, BrokerType: "console", ConsoleSession: session.Token}, session.ExpiresAt, err
		}, func(ctx context.Context, p broker.Principal) error {
			return identityClient.Logout(ctx, p.ConsoleSession)
		})
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
