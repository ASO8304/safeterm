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
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/identity"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
	"safeterm/internal/layout"
	"safeterm/internal/networkmanager"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func uid(name string) (uint32, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(u.Uid, 10, 32)
	return uint32(n), err
}
func run() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("network: manager requires root service identity")
	}
	console, err := uid("safeterm")
	if err != nil {
		return err
	}
	ssh, err := uid("safeterm-broker")
	if err != nil {
		return err
	}
	identityUID, err := uid(identity.UserName)
	if err != nil {
		return err
	}
	auditUID, err := uid("safeterm-audit")
	if err != nil {
		return err
	}
	if console == ssh || console == 0 || ssh == 0 {
		return fmt.Errorf("network: distinct non-root Broker identities required")
	}
	cfg, err := config.LoadConfig(layout.Generated)
	if err != nil {
		return err
	}
	if err = integrity.VerifyCatalogFiles(cfg, layout.Trust, filepath.Join(layout.Generated, "catalog.sig")); err != nil {
		return err
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		return err
	}
	sessions := identity.NewClient(identity.SocketPath, identityUID)
	sink, err := audit.NewUnixClient("/run/safeterm-audit/audit.sock", []uint32{auditUID})
	if err != nil {
		return err
	}
	lock, err := networkmanager.LockState("/", networkmanager.StatePath)
	if err != nil {
		return err
	}
	defer lock.Close()
	manager, err := networkmanager.New(networkmanager.Options{Root: "/", State: networkmanager.StatePath, Policy: func() (networkmanager.Policy, error) { return networkmanager.ReadPolicy("/") }, Interfaces: networkmanager.PhysicalInterfaces, Backend: &networkmanager.Netplan{Root: "/"}, Audit: sink, TrialLock: networkmanager.LifecycleLock})
	if err != nil {
		return err
	}
	defer manager.Close()
	policies := map[uint32]isolation.PeerPolicy{
		console: {AuthorizePeer: func(req *contract.ExecutionRequest, peer isolation.PeerIdentity) error {
			if req.Metadata.BrokerType != "console" {
				return identity.ErrDenied
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			session, err := sessions.Verify(ctx, req.Metadata.ConsoleSession, identity.Peer{UID: peer.UID, PID: peer.PID})
			if err != nil || session.UserID != req.Metadata.UserID {
				return identity.ErrDenied
			}
			req.Metadata.ClaimedRoles = append([]string(nil), session.Roles...)
			req.Metadata.SessionExpiresAt = session.ExpiresAt
			return nil
		}},
		ssh: {Authorize: func(req *contract.ExecutionRequest) error {
			if req.Metadata.BrokerType != "ssh" || req.Metadata.ConsoleSession != "" {
				return identity.ErrDenied
			}
			// Re-read roles so SSH identity removal/role revocation is effective even
			// for an already-open SSH connection. Credentials never cross this socket.
			data, err := networkmanager.ReadTrustedData("/", "etc/safeterm/access/ssh/users.json")
			if err != nil {
				return err
			}
			defer clear(data)
			var users broker.SSHIdentityConfig
			if err = networkmanager.DecodeJSON(data, &users); err != nil {
				return err
			}
			if _, err = broker.NewSSHAuthenticator(users, catalog.KnownRoles()); err != nil {
				return err
			}
			for _, u := range users.Users {
				if u.UserID == req.Metadata.UserID {
					req.Metadata.ClaimedRoles = append([]string(nil), u.Roles...)
					return nil
				}
			}
			return identity.ErrDenied
		}},
	}
	server, err := isolation.ListenUnixWithPeerPolicies(networkmanager.Socket, 0660, policies, manager)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}
