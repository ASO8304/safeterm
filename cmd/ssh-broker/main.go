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

	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/configmanager"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
	"safeterm/internal/layout"
)

func main() {
	configDir := flag.String("config", layout.Generated, "signed catalog directory")
	listenAddress := flag.String("listen", ":2222", "SSH listen address")
	hostKey := flag.String("host-key", "/etc/safeterm/ssh/ssh_host_ed25519_key", "SSH host private key")
	identities := flag.String("identities", "/etc/safeterm/ssh/users.json", "SSH password-hash and optional public-key identity mapping")
	gatewaySocket := flag.String("gateway-socket", "/run/safeterm-gateway/gateway.sock", "Node Gateway socket")
	gatewayUser := flag.String("gateway-user", "safeterm-gateway", "trusted Node Gateway user")
	runAsUser := flag.String("run-as-user", "safeterm-broker", "required Broker operating-system user")
	catalogPublicKey := flag.String("catalog-public-key", "", "trusted catalog Ed25519 public key")
	catalogSignature := flag.String("catalog-signature", "", "detached catalog signature")
	configManagers := flag.String("config-managers", "", "comma-separated Config Manager socket:user endpoints")
	flag.Parse()
	if err := run(*configDir, *listenAddress, *hostKey, *identities, *gatewaySocket, *gatewayUser, *runAsUser, *catalogPublicKey, *catalogSignature, *configManagers); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configDir, listenAddress, hostKeyPath, identitiesPath, gatewaySocket, gatewayUser, runAsUser, catalogPublicKey, catalogSignature, managerEndpoints string) error {
	current, err := user.Current()
	if err != nil || current.Username != runAsUser {
		return fmt.Errorf("ssh-broker: must run as OS user %q", runAsUser)
	}
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return err
	}
	if catalogPublicKey == "" {
		catalogPublicKey = layout.PublicKey(configDir)
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
	var configBroker *broker.ConfigBroker
	if strings.TrimSpace(managerEndpoints) != "" {
		services := []configmanager.Service{}
		for _, endpoint := range strings.Split(managerEndpoints, ",") {
			parts := strings.Split(strings.TrimSpace(endpoint), ":")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("ssh-broker: invalid Config Manager endpoint %q", endpoint)
			}
			uid, lookupErr := lookupUserUID(parts[1])
			if lookupErr != nil {
				return lookupErr
			}
			managerClient, clientErr := configmanager.NewUnixClient(parts[0], []uint32{uid})
			if clientErr != nil {
				return clientErr
			}
			services = append(services, managerClient)
		}
		configBroker, err = broker.NewConfigBroker(services)
		if err != nil {
			return err
		}
	}
	server, err := broker.NewSSHServerWithConfig(listenAddress, hostKey, authenticator, client, catalog, configBroker)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}

func lookupUserUID(name string) (uint32, error) {
	account, err := user.Lookup(name)
	if err != nil {
		return 0, fmt.Errorf("ssh-broker: resolve Config Manager user %q: %w", name, err)
	}
	value, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(value), nil
}
