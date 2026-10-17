//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"safeterm/internal/audit"
	"safeterm/internal/configmanager"
	"strconv"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("configwriter: requires root service identity")
	}
	if err := configmanager.ValidateWriterRegistry(); err != nil {
		return err
	}
	registry, err := configmanager.LoadRegistry("/etc/safeterm/config-manager/resources.json")
	if err != nil {
		return err
	}
	for _, r := range registry.Resources {
		if r.PrivilegedWrite {
			if err := configmanager.ValidateWriterPath(r.Path); err != nil {
				return err
			}
		}
	}
	peers := map[uint32]string{}
	for _, profile := range []string{"application", "database", "network"} {
		account, err := user.Lookup("safeterm-config-" + profile)
		if err != nil {
			return err
		}
		uid, err := strconv.ParseUint(account.Uid, 10, 32)
		if err != nil || uid == 0 {
			return fmt.Errorf("configwriter: invalid manager identity")
		}
		if _, exists := peers[uint32(uid)]; exists {
			return fmt.Errorf("configwriter: duplicate manager UID")
		}
		peers[uint32(uid)] = profile
	}
	account, err := user.Lookup("safeterm-audit")
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return err
	}
	sink, err := audit.NewUnixClient("/run/safeterm-audit/audit.sock", []uint32{uint32(uid)})
	if err != nil {
		return err
	}
	server, err := configmanager.NewFileWriterServer(registry, configmanager.WriterState, peers, sink)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx, configmanager.WriterSocket)
}
