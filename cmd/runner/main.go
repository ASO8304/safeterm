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

	"safeterm/internal/agent"
	"safeterm/internal/audit"
	"safeterm/internal/config"
	"safeterm/internal/execution"
	"safeterm/internal/integrity"
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
	outputLimit := flag.Int64("output-limit", 1<<20, "maximum combined stdout/stderr bytes per operation")
	streamChunk := flag.Int("stream-chunk", 16<<10, "maximum bytes per stream chunk")
	streamQueue := flag.Int("stream-queue", 64, "bounded stream frame queue depth")
	maxSessions := flag.Int("stream-max-sessions", 64, "maximum concurrent streaming sessions")
	idleTimeout := flag.Duration("stream-idle-timeout", 5*time.Minute, "maximum time without stream output")
	interruptGrace := flag.Duration("interrupt-grace", 2*time.Second, "grace period after interrupt")
	terminateGrace := flag.Duration("terminate-grace", 2*time.Second, "grace period after terminate")
	auditSocket := flag.String("audit-socket", "/run/safeterm-audit/audit.sock", "Audit Service socket")
	auditUser := flag.String("audit-user", "safeterm-audit", "trusted Audit Service user")
	catalogPublicKey := flag.String("catalog-public-key", "", "trusted catalog Ed25519 public key (defaults under config directory)")
	catalogSignature := flag.String("catalog-signature", "", "detached catalog signature (defaults under config directory)")
	flag.Parse()
	streamOptions := isolation.StreamOptions{ChunkBytes: *streamChunk, OutputBudget: *outputLimit, QueueDepth: *streamQueue, MaxSessions: *maxSessions, IdleTimeout: *idleTimeout, HistoryTTL: 5 * time.Minute}
	executionOptions := execution.Options{Timeout: *timeout, OutputLimit: *outputLimit, ChunkBytes: *streamChunk, InterruptGrace: *interruptGrace, TerminateGrace: *terminateGrace}
	if err := run(*configDir, config.RunnerProfile(*profileName), *gatewayUser, executionOptions, streamOptions, *auditSocket, *auditUser, *catalogPublicKey, *catalogSignature); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(configDir string, profileName config.RunnerProfile, gatewayUser string, executionOptions execution.Options, streamOptions isolation.StreamOptions, auditSocket, auditUser, catalogPublicKey, catalogSignature string) error {
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
	store, err := isolation.NewVerifiedStore(cfg, func(candidate *config.Config) error {
		return integrity.VerifyCatalogFiles(candidate, catalogPublicKey, catalogSignature)
	})
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
	processRunner, err := execution.NewWithOptions(executionOptions)
	if err != nil {
		return err
	}
	verifiedRunner, err := integrity.NewExecutor(profileConfig, profileName, processRunner)
	if err != nil {
		return err
	}
	var executor agent.Executor
	switch profile.ExecutorKind {
	case config.ExecutorProcess:
		executor = verifiedRunner
	case config.ExecutorSystemd:
		controllerPath, err := executablePath(profileConfig, profile.ControllerExecutableRef)
		if err != nil {
			return err
		}
		controller, err := execution.NewSystemctlControllerWithPrefix(controllerPath, profile.ControllerArgsPrefix, verifiedRunner)
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
	auditAccount, err := user.Lookup(auditUser)
	if err != nil {
		return fmt.Errorf("runner: resolve Audit Service user: %w", err)
	}
	auditUID, err := strconv.ParseUint(auditAccount.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("runner: parse Audit Service UID: %w", err)
	}
	auditClient, err := audit.NewUnixClient(auditSocket, []uint32{uint32(auditUID)})
	if err != nil {
		return err
	}
	handler, err := isolation.NewProfileHandlerWithSink(profileName, store, core, auditClient)
	if err != nil {
		return err
	}
	sessions, err := isolation.NewSessionManager(profileName, store, handler, auditClient, streamOptions)
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
	server, err := isolation.ListenUnixStreaming(profile.SocketPath, 0o660, []uint32{uint32(gatewayUID)}, handler, sessions)
	if err != nil {
		return err
	}
	defer server.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(ctx)
}

func executablePath(cfg *config.Config, id string) (string, error) {
	for _, executable := range cfg.Executables {
		if executable.ID == id {
			return executable.Path, nil
		}
	}
	return "", fmt.Errorf("runner: executable %q is missing from profile catalog", id)
}
