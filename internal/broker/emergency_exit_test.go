package broker

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

type stalledGateway struct {
	started    chan struct{}
	terminated bool
}

func (g *stalledGateway) Handle(ctx context.Context, _ *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	close(g.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (g *stalledGateway) StartStream(_ context.Context, _ *contract.ExecutionRequest) (*isolation.StreamSession, error) {
	close(g.started)
	return &isolation.StreamSession{ID: "stalled-session", Events: make(chan contract.StreamFrame)}, nil
}

func (g *stalledGateway) Terminate(_ context.Context, _ contract.ControlRequest) (contract.ControlResult, error) {
	g.terminated = true
	return contract.ControlResult{}, nil
}

func TestEmergencyExitCancelsStalledOperations(t *testing.T) {
	for _, operation := range []string{"system_info", "capture", "telnet"} {
		t.Run(operation, func(t *testing.T) {
			cfg := brokerTestConfig()
			if operation == "telnet" {
				source, err := config.LoadConfig("../defaults/catalog")
				if err != nil {
					t.Fatal(err)
				}
				for _, op := range source.Operations {
					if op.Name == "telnet" {
						cfg.Operations = append(cfg.Operations, op)
					}
				}
				cfg.Policies[1].Operations = append(cfg.Policies[1].Operations, "telnet")
			}
			catalog, _ := NewCatalog(cfg)
			g := &stalledGateway{started: make(chan struct{})}
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			var output bytes.Buffer
			shell, err := NewShell(g, catalog, Principal{UserID: "alice", Roles: []string{"viewer", "netops"}}, reader, &output)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			finished := make(chan error, 1)
			go func() { finished <- shell.Run(ctx) }()
			command := "execute system_info host\r"
			if operation == "telnet" {
				command = "execute telnet 127.0.0.1 23\r"
			}
			if operation == "capture" {
				command = "execute capture host=127.0.0.1 port=999\r"
			}
			if _, err := io.WriteString(writer, command); err != nil {
				t.Fatal(err)
			}
			select {
			case <-g.started:
			case <-ctx.Done():
				t.Fatal("operation did not start")
			}
			if _, err := writer.Write([]byte{0x1d}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-finished:
				if err != nil || !strings.Contains(output.String(), "session closed (Ctrl+])") {
					t.Fatalf("escape err=%v output=%q", err, output.String())
				}
			case <-ctx.Done():
				t.Fatal("emergency exit remained blocked")
			}
			if operation == "capture" && !g.terminated {
				t.Fatal("active stream not terminated")
			}
		})
	}
}

func TestEmergencyExitAtPrompt(t *testing.T) {
	catalog, _ := NewCatalog(brokerTestConfig())
	var output bytes.Buffer
	shell, _ := NewShell(&fakeGateway{}, catalog, Principal{UserID: "alice", Roles: []string{"viewer"}}, strings.NewReader("\x1d"), &output)
	if err := shell.Run(context.Background()); err != nil || !strings.Contains(output.String(), "session closed (Ctrl+])") {
		t.Fatalf("err=%v output=%q", err, output.String())
	}
}
