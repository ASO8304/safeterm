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
	"safeterm/internal/config"
	"safeterm/internal/isolation"
)

func main() {
	configDir := flag.String("config", "/etc/safeterm", "catalog directory")
	socketPath := flag.String("socket", "/run/safeterm-gateway/gateway.sock", "Gateway socket")
	brokerUser := flag.String("broker-user", "safeterm-broker", "allowed local Broker user")
	auditSocket := flag.String("audit-socket", "/run/safeterm-audit/audit.sock", "Audit Service socket")
	auditUser := flag.String("audit-user", "safeterm-audit", "trusted Audit Service user")
	flag.Parse()
	if err := run(*configDir, *socketPath, *brokerUser, *auditSocket, *auditUser); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configDir, socketPath, brokerUser, auditSocket, auditUser string) error {
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil || current.Username != "safeterm-gateway" {
		return fmt.Errorf("gateway: must run as OS user %q", "safeterm-gateway")
	}
	store, err := isolation.NewStore(cfg)
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
	server, err := isolation.ListenUnix(socketPath, 0o660, []uint32{uint32(brokerUID)}, gateway)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}
