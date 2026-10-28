//go:build linux

package broker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"safeterm/internal/configmanager"
)

func TestConfigEditorThroughAuthenticatedIPCForSSHAndConsole(t *testing.T) {
	for _, brokerType := range []string{"ssh", "console"} {
		t.Run(brokerType, func(t *testing.T) {
			s, path, out := editorFixture(t)
			manager := s.configs.clients[0]
			socket := filepath.Join(t.TempDir(), "config.sock")
			uid := uint32(os.Getuid())
			server, err := configmanager.ListenUnix(socket, 0600, []uint32{uid}, manager)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			defer server.Close()
			server.SetActorAuthorizer(func(a *configmanager.Actor, peerUID uint32, _ int32) error {
				if a.UserID != "alice" || a.BrokerType != brokerType || peerUID != uid {
					return fmt.Errorf("denied")
				}
				if brokerType == "console" && a.ConsoleSession != "authenticated-login" {
					return fmt.Errorf("login required")
				}
				a.Roles = []string{"viewer", "operator"}
				return nil
			})
			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx) }()
			client, err := configmanager.NewUnixClient(socket, []uint32{uid})
			if err != nil {
				t.Fatal(err)
			}
			s.configs, _ = NewConfigBroker([]configmanager.Service{client})
			s.principal.BrokerType = brokerType
			if brokerType == "console" {
				s.principal.ConsoleSession = "authenticated-login"
			}
			s.input = strings.NewReader("config options\redit Server\rset Port 9001\rnext\redit Logging\rset Level debug\rdiff\rsave\rexit\r")
			if err = s.Run(ctx); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			if !strings.Contains(string(data), "Port = 9001 ; port comment\r\n") || !strings.Contains(string(data), "Level = debug\r\n") {
				t.Fatalf("file=%q", data)
			}
			if !strings.Contains(out.String(), "Configuration saved.") || strings.Contains(out.String(), "error:") {
				t.Fatalf("terminal=%s", out)
			}
			cancel()
			if err = <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLogicalConfigMappingThroughAuthenticatedIPC(t *testing.T) {
	for _, brokerType := range []string{"ssh", "console"} {
		t.Run(brokerType, func(t *testing.T) {
			s, path, out := logicalConfigFixture(t)
			manager := s.configs.clients[0]
			socket := filepath.Join(t.TempDir(), "logical.sock")
			uid := uint32(os.Getuid())
			server, err := configmanager.ListenUnix(socket, 0600, []uint32{uid}, manager)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			defer server.Close()
			server.SetActorAuthorizer(func(a *configmanager.Actor, peerUID uint32, _ int32) error {
				if a.UserID != "alice" || a.BrokerType != brokerType || peerUID != uid {
					return fmt.Errorf("denied")
				}
				if brokerType == "console" && a.ConsoleSession != "authenticated-login" {
					return fmt.Errorf("login required")
				}
				a.Roles = []string{"viewer", "operator"}
				return nil
			})
			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx) }()
			client, err := configmanager.NewUnixClient(socket, []uint32{uid})
			if err != nil {
				t.Fatal(err)
			}
			s.configs, err = NewConfigBroker([]configmanager.Service{client})
			if err != nil {
				t.Fatal(err)
			}
			s.principal.BrokerType = brokerType
			if brokerType == "console" {
				s.principal.ConsoleSession = "authenticated-login"
			}
			s.input = strings.NewReader("config tunnel\rset Value_1 1400\rset Value_2 443\rshow\rdiff\rsave\rconfig application cat\rset enabled false\rsave\rexit\r")
			if err := s.Run(ctx); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			for _, part := range []string{"TCPMSS = 1400 ; preserve", "Port_2 = 443", "Enabled = false", "Unmanaged = untouched"} {
				if !strings.Contains(string(data), part) {
					t.Fatalf("missing %s: %q", part, data)
				}
			}
			if strings.Contains(out.String(), "error:") || strings.Contains(out.String(), "secret-sentinel") {
				t.Fatalf("bad terminal output %q", out)
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
