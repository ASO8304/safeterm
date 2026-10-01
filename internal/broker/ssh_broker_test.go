package broker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

func brokerTestConfig() *config.Config {
	minimum, maximum := 1, 65535
	return &config.Config{
		Version: "m12-test",
		Profiles: []config.ProfileDefinition{
			{Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/diagnostics.sock", RunAsUser: "diagnostics", MemoryLimitMiB: 64, ProcessLimit: 8},
			{Name: config.ProfileNetworkObserver, ExecutorKind: config.ExecutorProcess, SocketPath: "/run/network.sock", RunAsUser: "network", Capabilities: []string{"CAP_NET_RAW"}, MemoryLimitMiB: 64, ProcessLimit: 8},
		},
		Executables: []config.ExecutableDefinition{
			{ID: "uname", Path: "/usr/bin/uname", SHA256: strings.Repeat("a", 64), AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}},
			{ID: "tcpdump", Path: "/usr/bin/tcpdump", SHA256: strings.Repeat("b", 64), AllowedProfiles: []config.RunnerProfile{config.ProfileNetworkObserver}},
		},
		Operations: []config.OperationDefinition{
			{Name: "system_info", Description: "System information", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: "uname", ArgsTemplate: []string{"-a"}},
			{Name: "trace", Description: "Capture traffic", RunnerProfile: config.ProfileNetworkObserver, ExecutorKind: config.ExecutorProcess, ExecutableRef: "tcpdump", ArgsTemplate: []string{"port", "{port}"}, LongRunning: true, Parameters: []config.ParameterDefinition{{Name: "port", Type: config.ParamTypeInt, Required: true, Min: &minimum, Max: &maximum}}},
		},
		Roles: []config.RoleDefinition{{Name: "viewer"}, {Name: "netops"}},
		Policies: []config.PolicyDefinition{
			{Role: "viewer", Operations: []string{"system_info"}},
			{Role: "netops", Operations: []string{"system_info", "trace"}},
		},
	}
}

func testSigner(t *testing.T) ssh.Signer {
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

func testAuthenticator(t *testing.T, catalog *Catalog, userSigner ssh.Signer) *SSHAuthenticator {
	t.Helper()
	authenticator, err := NewSSHAuthenticator(SSHIdentityConfig{Users: []SSHUser{{
		UserID: "alice", Roles: []string{"netops"},
		AuthorizedKeys: []string{string(ssh.MarshalAuthorizedKey(userSigner.PublicKey()))},
	}}}, catalog.KnownRoles())
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

// TEST-BROKER-001, TEST-BROKER-002
func TestAuthenticatorBindsKeyUsernameAndConfiguredRoles(t *testing.T) {
	catalog, err := NewCatalog(brokerTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	key := testSigner(t)
	authenticator := testAuthenticator(t, catalog, key)
	metadata := connectionMetadata{user: "alice"}
	permissions, err := authenticator.Authenticate(metadata, key.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	principal, err := principalFromPermissions(permissions)
	if err != nil || principal.UserID != "alice" || len(principal.Roles) != 1 || principal.Roles[0] != "netops" {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
	if _, err := authenticator.Authenticate(connectionMetadata{user: "mallory"}, key.PublicKey()); err == nil {
		t.Fatal("same key accepted under a different username")
	}
	if _, err := authenticator.Authenticate(metadata, testSigner(t).PublicKey()); err == nil {
		t.Fatal("unknown key accepted")
	}
}

type connectionMetadata struct{ user string }

func (m connectionMetadata) User() string        { return m.user }
func (connectionMetadata) SessionID() []byte     { return []byte("session") }
func (connectionMetadata) ClientVersion() []byte { return []byte("client") }
func (connectionMetadata) ServerVersion() []byte { return []byte("server") }
func (connectionMetadata) RemoteAddr() net.Addr  { return testAddr("remote") }
func (connectionMetadata) LocalAddr() net.Addr   { return testAddr("local") }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

type fakeGateway struct {
	mu         sync.Mutex
	requests   []*contract.ExecutionRequest
	terminated []contract.ControlRequest
	events     chan contract.StreamFrame
}

func (f *fakeGateway) Handle(_ context.Context, request *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	return &contract.ExecutionResponse{RequestID: request.RequestID, Result: contract.ResultSuccess, Stdout: "Linux safe\n", ExitCode: 0, StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC()}, nil
}

func (f *fakeGateway) StartStream(_ context.Context, request *contract.ExecutionRequest) (*isolation.StreamSession, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	f.events = make(chan contract.StreamFrame, 4)
	f.events <- contract.StreamFrame{Version: contract.StreamProtocolVersion, Type: contract.FrameSessionStarted, RequestID: request.RequestID, SessionID: "opaque-session", State: contract.SessionRunning, OccurredAt: time.Now().UTC()}
	f.events <- contract.StreamFrame{Version: contract.StreamProtocolVersion, Type: contract.FrameStdout, RequestID: request.RequestID, SessionID: "opaque-session", Sequence: 1, Data: []byte("packet\x1b]52;c;unsafe\a\n"), State: contract.SessionRunning, OccurredAt: time.Now().UTC()}
	return &isolation.StreamSession{ID: "opaque-session", RequestID: request.RequestID, Events: f.events}, nil
}

func (f *fakeGateway) Terminate(_ context.Context, request contract.ControlRequest) (contract.ControlResult, error) {
	f.mu.Lock()
	f.terminated = append(f.terminated, request)
	response := &contract.ExecutionResponse{RequestID: f.requests[len(f.requests)-1].RequestID, Result: contract.ResultExecutionError, ExitCode: 130, StartedAt: time.Now().Add(-time.Second).UTC(), EndedAt: time.Now().UTC()}
	f.events <- contract.StreamFrame{Version: contract.StreamProtocolVersion, Type: contract.FrameSessionExited, RequestID: response.RequestID, SessionID: request.SessionID, Sequence: 2, State: contract.SessionExited, Response: response, OccurredAt: time.Now().UTC()}
	close(f.events)
	f.mu.Unlock()
	return contract.ControlResult{Version: contract.StreamProtocolVersion, SessionID: request.SessionID, Status: contract.ControlTerminating}, nil
}

// TEST-BROKER-003, TEST-BROKER-004, TEST-BROKER-005, TEST-BROKER-013
func TestRestrictedShellFiltersCatalogInjectsIdentityAndMapsInterrupt(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	client := &fakeGateway{}
	output := &bytes.Buffer{}
	shell, err := NewShell(client, catalog, Principal{UserID: "alice", Roles: []string{"netops"}}, strings.NewReader(""), output)
	if err != nil {
		t.Fatal(err)
	}
	input := make(chan byte, 1)
	done := make(chan error, 1)
	go func() {
		_, err := shell.executeLine(context.Background(), "run trace port=53", input)
		done <- err
	}()
	input <- 0x03
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 || client.requests[0].Metadata.UserID != "alice" || client.requests[0].Metadata.BrokerType != "ssh" || strings.Join(client.requests[0].Metadata.ClaimedRoles, ",") != "netops" {
		t.Fatalf("request did not use authenticated identity: %+v", client.requests)
	}
	if len(client.terminated) != 1 || client.terminated[0].Action != contract.ControlTerminate || client.terminated[0].Owner != (contract.SessionOwner{BrokerType: "ssh", UserID: "alice"}) {
		t.Fatalf("interrupt was not semantic termination: %+v", client.terminated)
	}
	if strings.Contains(output.String(), "\x1b") || strings.Contains(output.String(), "\a") {
		t.Fatalf("terminal control sequence reached client output: %q", output.String())
	}
	viewer, _ := NewShell(client, catalog, Principal{UserID: "bob", Roles: []string{"viewer"}}, strings.NewReader(""), io.Discard)
	if _, err := viewer.executeLine(context.Background(), "run trace port=53", input); err == nil {
		t.Fatal("unauthorized operation was accepted by the Broker catalog")
	}
}

// TEST-BROKER-006, TEST-BROKER-007, TEST-BROKER-008
func TestSSHServerProvidesPTYRestrictedShellAndRejectsExec(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	userSigner := testSigner(t)
	server, err := NewSSHServer("unused", testSigner(t), testAuthenticator(t, catalog, userSigner), &fakeGateway{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.ServeListener(ctx, listener) }()
	clientConfig := &ssh.ClientConfig{User: "alice", Auth: []ssh.AuthMethod{ssh.PublicKeys(userSigner)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second}
	client, err := ssh.Dial("tcp", listener.Addr().String(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	execSession, _ := client.NewSession()
	if err := execSession.Run("uname -a"); err == nil {
		t.Fatal("SSH exec request was accepted")
	}
	_ = execSession.Close()
	_ = client.Close()

	client, err = ssh.Dial("tcp", listener.Addr().String(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	stdin, _ := session.StdinPipe()
	stdout, _ := session.StdoutPipe()
	if err := session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(stdin, "whoami\rexit\r")
	result, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err != nil {
		t.Fatal(err)
	}
	text := string(result)
	if !strings.Contains(text, "SafeTerm restricted shell") || !strings.Contains(text, "user: alice") || !strings.Contains(text, "roles: netops") {
		t.Fatalf("unexpected terminal output: %q", text)
	}
	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SSH server did not stop")
	}
}

// TEST-BROKER-009
func TestSSHIdentityConfigurationFailsClosed(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	keyText := string(ssh.MarshalAuthorizedKey(testSigner(t).PublicKey()))
	cases := []SSHIdentityConfig{
		{},
		{Users: []SSHUser{{UserID: "alice", Roles: []string{"admin"}, AuthorizedKeys: []string{keyText}}}},
		{Users: []SSHUser{{UserID: "bad user", Roles: []string{"viewer"}, AuthorizedKeys: []string{keyText}}}},
		{Users: []SSHUser{{UserID: "alice", Roles: []string{"viewer"}, AuthorizedKeys: []string{"invalid"}}}},
	}
	for index, candidate := range cases {
		if _, err := NewSSHAuthenticator(candidate, catalog.KnownRoles()); err == nil {
			t.Fatalf("case %d unexpectedly accepted", index)
		}
	}
	duplicate := SSHIdentityConfig{Users: []SSHUser{
		{UserID: "alice", Roles: []string{"viewer"}, AuthorizedKeys: []string{keyText}},
		{UserID: "bob", Roles: []string{"viewer"}, AuthorizedKeys: []string{keyText}},
	}}
	if _, err := NewSSHAuthenticator(duplicate, catalog.KnownRoles()); err == nil || !strings.Contains(fmt.Sprint(err), "assigned more than once") {
		t.Fatal("key reuse across identities was accepted")
	}
}

// TEST-BROKER-012
func TestSSHSecurityFilesAreStrictBoundedAndNotSymlinks(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	userSigner := testSigner(t)
	identityConfig := SSHIdentityConfig{Users: []SSHUser{{UserID: "alice", Roles: []string{"viewer"}, AuthorizedKeys: []string{string(ssh.MarshalAuthorizedKey(userSigner.PublicKey()))}}}}
	identityData, _ := json.Marshal(identityConfig)
	directory := t.TempDir()
	identities := filepath.Join(directory, "users.json")
	if err := os.WriteFile(identities, identityData, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHAuthenticator(identities, catalog.KnownRoles()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identities, []byte(`{"users":[],"unexpected":true}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHAuthenticator(identities, catalog.KnownRoles()); err == nil {
		t.Fatal("unknown identity field accepted")
	}
	identityLink := filepath.Join(directory, "users-link.json")
	if err := os.Symlink(identities, identityLink); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHAuthenticator(identityLink, catalog.KnownRoles()); err == nil {
		t.Fatal("symlink identity file accepted")
	}

	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	hostKey := filepath.Join(directory, "host-key")
	if err := os.WriteFile(hostKey, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHHostKey(hostKey); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hostKey, 0o660); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHHostKey(hostKey); err == nil {
		t.Fatal("group-writable host key accepted")
	}
	hostKeyLink := filepath.Join(directory, "host-key-link")
	if err := os.Symlink(hostKey, hostKeyLink); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHHostKey(hostKeyLink); err == nil {
		t.Fatal("symlink host key accepted")
	}
}
