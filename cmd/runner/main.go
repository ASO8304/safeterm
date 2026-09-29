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
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/execution"
	"safeterm/internal/isolation"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

func main() {
	configDir := flag.String("config", "/etc/safeterm", "catalog directory")
	profileName := flag.String("profile", "", "runner profile")
	gatewayUser := flag.String("gateway-user", "safeterm-gateway", "allowed local Gateway user")
	timeout := flag.Duration("timeout", 30*time.Second, "maximum operation runtime")
	systemctlPath := flag.String("systemctl", "/usr/bin/systemctl", "absolute systemctl path")
	flag.Parse()
	if err := run(*configDir, config.RunnerProfile(*profileName), *gatewayUser, *timeout, *systemctlPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configDir string, profileName config.RunnerProfile, gatewayUser string, timeout time.Duration, systemctlPath string) error {
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return err
	}
	store, err := isolation.NewStore(cfg)
	if err != nil {
		return err
	}
	profile, ok := store.Current().Profile(profileName)
	if !ok {
		return fmt.Errorf("runner: unknown profile %q", profileName)
	}
	currentUser, err := user.Current()
	if err != nil || currentUser.Username != profile.RunAsUser {
		return fmt.Errorf("runner: profile %q must run as OS user %q", profileName, profile.RunAsUser)
	}
	profileConfig, err := store.Current().ProfileConfig(profileName)
	if err != nil {
		return err
	}
	translator, err := translation.New(profileConfig)
	if err != nil {
		return err
	}
	processRunner, err := execution.New(timeout)
	if err != nil {
		return err
	}
	var executor agent.Executor
	switch profile.ExecutorKind {
	case config.ExecutorProcess:
		executor = processRunner
	case config.ExecutorSystemd:
		controller, err := execution.NewSystemctlController(systemctlPath, processRunner)
		if err != nil {
			return err
		}
		executor, err = execution.NewSystemdExecutor(profileConfig, controller)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("runner: unsupported executor kind %q", profile.ExecutorKind)
	}
	core := agent.NewCore(validation.New(profileConfig), policy.New(profileConfig), translator, executor)
	handler, err := isolation.NewProfileHandler(profileName, store, core)
	if err != nil {
		return err
	}
	gatewayAccount, err := user.Lookup(gatewayUser)
	if err != nil {
		return fmt.Errorf("runner: resolve Gateway user: %w", err)
	}
	gatewayUID, err := strconv.ParseUint(gatewayAccount.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("runner: parse Gateway UID: %w", err)
	}
	server, err := isolation.ListenUnix(profile.SocketPath, 0o660, []uint32{uint32(gatewayUID)}, handler)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}
