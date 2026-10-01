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
	"syscall"

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
)

func main() {
	configDir := flag.String("config", "/etc/safeterm", "signed catalog directory")
	listenAddress := flag.String("listen", ":2222", "SSH listen address")
	hostKey := flag.String("host-key", "/etc/safeterm/ssh/ssh_host_ed25519_key", "SSH host private key")
	identities := flag.String("identities", "/etc/safeterm/ssh/users.json", "SSH public-key identity mapping")
	gatewaySocket := flag.String("gateway-socket", "/run/safeterm-gateway/gateway.sock", "Node Gateway socket")
	gatewayUser := flag.String("gateway-user", "safeterm-gateway", "trusted Node Gateway user")
	runAsUser := flag.String("run-as-user", "safeterm-broker", "required Broker operating-system user")
	catalogPublicKey := flag.String("catalog-public-key", "", "trusted catalog Ed25519 public key")
	catalogSignature := flag.String("catalog-signature", "", "detached catalog signature")
	flag.Parse()
	if err := run(*configDir, *listenAddress, *hostKey, *identities, *gatewaySocket, *gatewayUser, *runAsUser, *catalogPublicKey, *catalogSignature); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configDir, listenAddress, hostKeyPath, identitiesPath, gatewaySocket, gatewayUser, runAsUser, catalogPublicKey, catalogSignature string) error {
	current, err := user.Current()
	if err != nil || current.Username != runAsUser {
		return fmt.Errorf("ssh-broker: must run as OS user %q", runAsUser)
	}
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
	if err := integrity.VerifyCatalogFiles(cfg, catalogPublicKey, catalogSignature); err != nil {
		return err
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		return err
	}
	authenticator, err := broker.LoadSSHAuthenticator(identitiesPath, catalog.KnownRoles())
	if err != nil {
		return err
	}
	hostKey, err := broker.LoadSSHHostKey(hostKeyPath)
	if err != nil {
		return err
	}
	gatewayAccount, err := user.Lookup(gatewayUser)
	if err != nil {
		return fmt.Errorf("ssh-broker: resolve Gateway user: %w", err)
	}
	gatewayUID, err := strconv.ParseUint(gatewayAccount.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("ssh-broker: parse Gateway UID: %w", err)
	}
	client, err := isolation.NewUnixClient(gatewaySocket, []uint32{uint32(gatewayUID)})
	if err != nil {
		return err
	}
	server, err := broker.NewSSHServer(listenAddress, hostKey, authenticator, client, catalog)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}
