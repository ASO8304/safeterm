package broker

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"safeterm/internal/networkmanager"
)

var terminalPattern = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

const maxSSHConnections = 128

type SSHServer struct {
	network networkmanager.API
	address string
	config  *ssh.ServerConfig
	client  GatewayClient
	catalog *Catalog
	configs *ConfigBroker
	mu      sync.Mutex
	active  map[net.Conn]bool
	slots   chan struct{}
	wg      sync.WaitGroup
}

func NewSSHServer(address string, hostKey ssh.Signer, authenticator *SSHAuthenticator, client GatewayClient, catalog *Catalog) (*SSHServer, error) {
	return NewSSHServerWithConfig(address, hostKey, authenticator, client, catalog, nil)
}

func NewSSHServerWithConfig(address string, hostKey ssh.Signer, authenticator *SSHAuthenticator, client GatewayClient, catalog *Catalog, configs *ConfigBroker) (*SSHServer, error) {
	if address == "" || hostKey == nil || authenticator == nil || client == nil || catalog == nil {
		return nil, fmt.Errorf("broker: SSH address, host key, authenticator, Gateway client, and catalog are required")
	}
	serverConfig := &ssh.ServerConfig{
		NoClientAuth: false,
		MaxAuthTries: 3,
		PublicKeyAuthAlgorithms: []string{
			ssh.KeyAlgoED25519,
			ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
			ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512,
		},

		ServerVersion: "SSH-2.0-SafeTerm",
	}
	if len(authenticator.byFingerprint) > 0 {
		serverConfig.PublicKeyCallback = authenticator.Authenticate
	}
	if len(authenticator.byPassword) > 0 {
		serverConfig.PasswordCallback = authenticator.AuthenticatePassword
	}
	serverConfig.AddHostKey(hostKey)
	return &SSHServer{address: address, config: serverConfig, client: client, catalog: catalog, configs: configs, active: map[net.Conn]bool{}, slots: make(chan struct{}, maxSSHConnections)}, nil
}

func LoadSSHHostKey(path string) (ssh.Signer, error) {
	key, err := readSecurityFile(path, "SSH host key", 64<<10, 0o027)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("broker: parse SSH host key: %w", err)
	}
	return signer, nil
}

func (s *SSHServer) Serve(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("broker: listen on %q: %w", s.address, err)
	}
	return s.ServeListener(ctx, listener)
}

func (s *SSHServer) ServeListener(ctx context.Context, listener net.Listener) error {
	if s == nil || ctx == nil || listener == nil {
		return fmt.Errorf("broker: invalid SSH server")
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
			s.closeActive()
		case <-done:
		}
	}()
	defer close(done)
	defer s.wg.Wait()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("broker: accept SSH connection: %w", err)
		}
		select {
		case s.slots <- struct{}{}:
		default:
			_ = connection.Close()
			continue
		}
		s.mu.Lock()
		s.active[connection] = true
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-s.slots }()
			defer func() {
				s.mu.Lock()
				delete(s.active, connection)
				s.mu.Unlock()
				_ = connection.Close()
			}()
			s.handleConnection(ctx, connection)
		}()
	}
}

func (s *SSHServer) closeActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for connection := range s.active {
		_ = connection.Close()
	}
}

func (s *SSHServer) handleConnection(ctx context.Context, connection net.Conn) {
	_ = connection.SetDeadline(time.Now().Add(15 * time.Second))
	serverConnection, channels, requests, err := ssh.NewServerConn(connection, s.config)
	if err != nil {
		return
	}
	defer serverConnection.Close()
	_ = connection.SetDeadline(time.Time{})
	principal, err := principalFromPermissions(serverConnection.Permissions)
	if err != nil {
		return
	}
	go rejectGlobalRequests(requests)
	acceptedSession := false
	for channelRequest := range channels {
		if channelRequest.ChannelType() != "session" {
			_ = channelRequest.Reject(ssh.UnknownChannelType, "SafeTerm accepts session channels only")
			continue
		}
		if acceptedSession {
			_ = channelRequest.Reject(ssh.ResourceShortage, "SafeTerm permits one terminal per SSH connection")
			continue
		}
		channel, channelRequests, err := channelRequest.Accept()
		if err != nil {
			continue
		}
		acceptedSession = true
		go s.handleSession(ctx, channel, channelRequests, principal)
	}
}

func rejectGlobalRequests(requests <-chan *ssh.Request) {
	for request := range requests {
		if request.WantReply {
			_ = request.Reply(false, nil)
		}
	}
}

func (s *SSHServer) handleSession(ctx context.Context, channel ssh.Channel, requests <-chan *ssh.Request, principal Principal) {
	defer channel.Close()
	ptyAccepted := false
	shellStarted := false
	shellDone := make(chan error, 1)
	for {
		select {
		case <-ctx.Done():
			return
		case <-shellDone:
			_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			return
		case request, ok := <-requests:
			if !ok {
				return
			}
			accepted := false
			switch request.Type {
			case "pty-req":
				accepted = !ptyAccepted && !shellStarted && validPTYRequest(request.Payload)
				ptyAccepted = ptyAccepted || accepted
			case "window-change":
				accepted = ptyAccepted && validWindowChange(request.Payload)
			case "shell":
				accepted = ptyAccepted && !shellStarted && len(request.Payload) == 0
				if accepted {
					shellStarted = true
					shell, err := NewShellWithConfig(s.client, s.catalog, principal, channel, channel, s.configs)
					if err != nil {
						accepted = false
						shellStarted = false
					} else {
						shell.SetNetworkClient(s.network)
						go func() { shellDone <- shell.Run(ctx) }()
					}
				}
			}
			if request.WantReply {
				_ = request.Reply(accepted, nil)
			}
		}
	}
}

func validPTYRequest(payload []byte) bool {
	if len(payload) < 4 || len(payload) > 8192 {
		return false
	}
	termLength := int(binary.BigEndian.Uint32(payload[:4]))
	if termLength < 1 || termLength > 64 || len(payload) < 4+termLength+20 {
		return false
	}
	term := string(payload[4 : 4+termLength])
	return terminalPattern.MatchString(term)
}

func validWindowChange(payload []byte) bool {
	if len(payload) != 16 {
		return false
	}
	columns := binary.BigEndian.Uint32(payload[0:4])
	rows := binary.BigEndian.Uint32(payload[4:8])
	return columns > 0 && rows > 0 && columns <= 1000 && rows <= 1000
}

// SetNetworkClient must be called before serving connections.
func (s *SSHServer) SetNetworkClient(client networkmanager.API) { s.network = client }
