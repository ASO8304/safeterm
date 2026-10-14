//go:build linux

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
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/identity"
	"safeterm/internal/integrity"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func uid(name string) (uint32, error) {
	u, e := user.Lookup(name)
	if e != nil {
		return 0, e
	}
	v, e := strconv.ParseUint(u.Uid, 10, 32)
	return uint32(v), e
}
func run() error {
	dir := flag.String("config", "/etc/safeterm", "signed catalog")
	users := flag.String("users", "/etc/safeterm/console/users.json", "root-managed internal Console users")
	socket := flag.String("socket", identity.SocketPath, "Identity Unix socket")
	ttl := flag.Duration("session-ttl", 30*time.Minute, "absolute Console session lifetime (1m to 12h)")
	flag.Parse()
	if os.Geteuid() == 0 {
		return fmt.Errorf("identity: must run as isolated safeterm-identity user")
	}
	current, e := user.Current()
	if e != nil || current.Username != identity.UserName {
		return fmt.Errorf("identity: incorrect OS user")
	}
	cfg, e := config.LoadConfig(*dir)
	if e != nil {
		return e
	}
	if e = integrity.VerifyCatalogFiles(cfg, filepath.Join(*dir, "trust/catalog-ed25519.pub"), filepath.Join(*dir, "catalog.sig")); e != nil {
		return e
	}
	catalog, e := broker.NewCatalog(cfg)
	if e != nil {
		return e
	}
	auditUID, e := uid("safeterm-audit")
	if e != nil {
		return e
	}
	sink, e := audit.NewUnixClient("/run/safeterm-audit/audit.sock", []uint32{auditUID})
	if e != nil {
		return e
	}
	group, e := user.LookupGroup(identity.UserName)
	if e != nil {
		return e
	}
	gid, e := strconv.ParseUint(group.Gid, 10, 32)
	if e != nil {
		return e
	}
	if e = identity.CheckRootManaged(*users, uint32(gid)); e != nil {
		return e
	}
	service, e := identity.New(*users, catalog.KnownRoles(), *ttl, sink, cfg.Version)
	if e != nil {
		return e
	}
	consoleUID, e := uid("safeterm")
	if e != nil {
		return e
	}
	var verifiers []uint32
	for _, name := range []string{"safeterm-gateway", "safeterm-config-application", "safeterm-config-database", "safeterm-config-network"} {
		v, e := uid(name)
		if e != nil {
			return e
		}
		verifiers = append(verifiers, v)
	}
	server, e := identity.Listen(*socket, service, consoleUID, verifiers)
	if e != nil {
		return e
	}
	defer server.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return server.Serve(ctx)
}
