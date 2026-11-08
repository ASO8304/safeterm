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

	"golang.org/x/sys/unix"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/identity"
	"safeterm/internal/integrity"
	"safeterm/internal/layout"
	"safeterm/internal/passwordchange"
)

func uid(name string) (uint32, error) {
	u, e := user.Lookup(name)
	if e != nil {
		return 0, e
	}
	n, e := strconv.ParseUint(u.Uid, 10, 32)
	return uint32(n), e
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return fmt.Errorf("password manager requires root service identity")
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		return err
	}
	if len(os.Args) == 3 && os.Args[1] == "--linux-password-child" {
		return passwordchange.LinuxPasswordChild(os.Args[2])
	}
	if len(os.Args) != 1 {
		return fmt.Errorf("password manager: unsupported arguments")
	}
	cfg, err := config.LoadConfig(layout.Generated)
	if err != nil {
		return err
	}
	if err := integrity.VerifyCatalogFiles(cfg, layout.Trust, filepath.Join(layout.Generated, "catalog.sig")); err != nil {
		return err
	}
	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		return err
	}
	ssh, err := uid("safeterm-broker")
	if err != nil {
		return err
	}
	console, err := uid("safeterm")
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
	client := identity.NewClient(identity.SocketPath, identityUID)
	sink, err := audit.NewUnixClient("/run/safeterm-audit/audit.sock", []uint32{auditUID})
	if err != nil {
		return err
	}
	service, err := passwordchange.New(passwordchange.Options{Root: "/", SSHUID: ssh, ConsoleUID: console, Roles: catalog.KnownRoles(), Audit: sink,
		Verify: func(ctx context.Context, token string, p passwordchange.Peer) (identity.Session, error) {
			return client.Verify(ctx, token, identity.Peer{UID: p.UID, PID: p.PID})
		}})
	if err != nil {
		return err
	}
	locals, err := identity.LocalUIDs(identity.LocalUsersPath)
	if err != nil {
		return err
	}
	allowed := append([]uint32{ssh, console}, locals...)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return passwordchange.Serve(ctx, passwordchange.Socket, service, allowed)
}
