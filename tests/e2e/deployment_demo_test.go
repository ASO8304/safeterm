//go:build linux

package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"

	"safeterm/internal/agent"
	"safeterm/internal/audit"
	"safeterm/internal/broker"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/execution"
	"safeterm/internal/integrity"
	"safeterm/internal/isolation"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

// TEST-E2E-001..004: REQ-015 AC-8 through AC-10.
// This test crosses the real SSH, authenticated Unix IPC, Gateway, Runner,
// integrity, execution, and Audit boundaries and executes /usr/bin/uname.
func TestDeployedDataPathFromSSHToVerifiedExecutionAndAudit(t *testing.T) {
	t.Run("public_key", func(t *testing.T) { deployedDataPath(t, false) })
	t.Run("password_only", func(t *testing.T) { deployedDataPath(t, true) })
}

func deployedDataPath(t *testing.T, passwordOnly bool) {
	cfg := demoConfig(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := integrity.SignCatalog(cfg, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	store, err := isolation.NewVerifiedStore(cfg, func(candidate *config.Config) error {
		return integrity.VerifyCatalog(candidate, publicKey, signature)
	})
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	uid := uint32(os.Getuid())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	auditStore, err := audit.OpenFileStore(filepath.Join(directory, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auditStore.Close() })
	auditSocket := filepath.Join(directory, "audit.sock")
	auditServer, err := audit.ListenUnix(auditSocket, 0o600, map[uint32][]audit.Producer{uid: {
		{Component: audit.ComponentGateway},
		{Component: audit.ComponentRunner, Profile: string(config.ProfileDiagnostics)},
	}}, auditStore)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auditServer.Close() })
	go func() { _ = auditServer.Serve(ctx) }()
	auditClient, err := audit.NewUnixClient(auditSocket, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}

	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	processRunner, err := execution.New(10 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	verifiedRunner, err := integrity.NewExecutor(cfg, config.ProfileDiagnostics, processRunner)
	if err != nil {
		t.Fatal(err)
	}
	core := agent.NewCore(validation.New(cfg), policy.New(cfg), translator, verifiedRunner)
	handler, err := isolation.NewProfileHandlerWithSink(config.ProfileDiagnostics, store, core, auditClient)
	if err != nil {
		t.Fatal(err)
	}
	streamOptions := isolation.DefaultStreamOptions()
	streamOptions.IdleTimeout = 5 * time.Second
	sessions, err := isolation.NewSessionManager(config.ProfileDiagnostics, store, handler, auditClient, streamOptions)
	if err != nil {
		t.Fatal(err)
	}
	runnerSocket := cfg.Profiles[0].SocketPath
	runnerServer, err := isolation.ListenUnixStreaming(runnerSocket, 0o600, []uint32{uid}, handler, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runnerServer.Close() })
	go func() { _ = runnerServer.Serve(ctx) }()
	runnerClient, err := isolation.NewUnixClient(runnerSocket, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}

	gateway, err := isolation.NewGatewayWithSink(store, map[config.RunnerProfile]isolation.Client{config.ProfileDiagnostics: runnerClient}, auditClient)
	if err != nil {
		t.Fatal(err)
	}
	gatewaySocket := filepath.Join(directory, "gateway.sock")
	gatewayServer, err := isolation.ListenUnixStreamingWithPeerPolicies(gatewaySocket, 0o600, map[uint32]isolation.PeerPolicy{uid: {
		AllowStreaming: true,
		Authorize: func(request *contract.ExecutionRequest) error {
			if request == nil || request.Metadata.BrokerType != "ssh" {
				return context.Canceled
			}
			return nil
		},
	}}, gateway, gateway)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gatewayServer.Close() })
	go func() { _ = gatewayServer.Serve(ctx) }()
	gatewayClient, err := isolation.NewUnixClient(gatewaySocket, []uint32{uid})
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := broker.NewCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner := newSigner(t)
	clientSigner := newSigner(t)
	identity := broker.SSHUser{UserID: "alice", Roles: []string{"viewer"}, AuthorizedKeys: []string{strings.TrimSpace(string(ssh.MarshalAuthorizedKey(clientSigner.PublicKey())))}}
	authMethod := ssh.PublicKeys(clientSigner)
	if passwordOnly {
		hash, err := bcrypt.GenerateFromPassword([]byte("e2e-only-password"), broker.PasswordHashCost)
		if err != nil {
			t.Fatal(err)
		}
		identity.AuthorizedKeys = nil
		identity.PasswordHash = string(hash)
		authMethod = ssh.Password("e2e-only-password")
	}
	authenticator, err := broker.NewSSHAuthenticator(broker.SSHIdentityConfig{Users: []broker.SSHUser{identity}}, catalog.KnownRoles())
	if err != nil {
		t.Fatal(err)
	}
	sshServer, err := broker.NewSSHServer("127.0.0.1:0", hostSigner, authenticator, gatewayClient, catalog)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- sshServer.ServeListener(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-serverDone:
		case <-time.After(2 * time.Second):
			t.Error("SSH server did not stop")
		}
	})

	clientConfig := &ssh.ClientConfig{
		User: "alice", Auth: []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.FixedHostKey(hostSigner.PublicKey()), Timeout: 5 * time.Second,
	}
	if passwordOnly {
		for _, rejected := range []ssh.AuthMethod{ssh.Password("incorrect-password"), ssh.PublicKeys(clientSigner)} {
			bad := *clientConfig
			bad.Auth = []ssh.AuthMethod{rejected}
			client, err := ssh.Dial("tcp", listener.Addr().String(), &bad)
			if err == nil {
				client.Close()
				t.Fatal("password-only server accepted invalid credentials or an unconfigured key")
			}
		}
	}
	output := runRestrictedShell(t, listener.Addr().String(), clientConfig, "whoami\rexecute ?\rsystem_info host\rexit\r")
	for _, expected := range []string{"Authenticated as alice", "roles: viewer", "system", "result: success", "Linux"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("terminal output missing %q:\n%s", expected, output)
		}
	}

	entries, err := audit.ReadFile(filepath.Join(directory, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("audit entries=%d, want 4", len(entries))
	}
	requestID := entries[0].Event.RequestID
	for _, entry := range entries {
		if entry.Event.RequestID != requestID || entry.Event.UserID != "alice" || entry.Event.Operation != "system_info" {
			t.Fatalf("uncorrelated audit entry: %+v", entry.Event)
		}
	}

	connection, err := ssh.Dial("tcp", listener.Addr().String(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	session, err := connection.NewSession()
	if err != nil {
		connection.Close()
		t.Fatal(err)
	}
	if err := session.Run("uname -a"); err == nil {
		t.Fatal("raw SSH exec was accepted")
	}
	session.Close()
	connection.Close()
}

func demoConfig(t *testing.T) *config.Config {
	t.Helper()
	path := "/usr/bin/uname"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return &config.Config{
		Version:     "m15-e2e",
		Profiles:    []config.ProfileDefinition{{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: filepath.Join(t.TempDir(), "runner.sock"), RunAsUser: "safeterm-diagnostics", MemoryLimitMiB: 64, ProcessLimit: 8}},
		Executables: []config.ExecutableDefinition{{ID: "uname", Path: path, SHA256: hex.EncodeToString(digest[:]), AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}}},
		Operations:  []config.OperationDefinition{{Name: "system_info", Description: "Show host information", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "uname", ArgsTemplate: []string{"-a"}, Parameters: []config.ParameterDefinition{}}},
		Roles:       []config.RoleDefinition{{Name: "viewer", Description: "Viewer"}},
		Policies:    []config.PolicyDefinition{{Role: "viewer", Operations: []string{"system_info"}}},
	}
}

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func runRestrictedShell(t *testing.T, address string, config *ssh.ClientConfig, commands string) string {
	t.Helper()
	connection, err := ssh.Dial("tcp", address, config)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	session, err := connection.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 80, 24, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, commands); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	data, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err != nil {
		t.Fatal(err)
	}
	return string(data)
}
