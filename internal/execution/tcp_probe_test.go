package execution

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

func tcpExecutorFixture(t *testing.T) *TCPProbeExecutor {
	t.Helper()
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewTCPProbeExecutor(cfg, &recordingExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestTCPProbeConnectsAndClosesWithoutApplicationData(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		data := make([]byte, 1)
		n, err := conn.Read(data)
		if n != 0 || err != io.EOF {
			done <- errors.New("TCP probe sent data or left the connection open")
			return
		}
		done <- nil
	}()
	e := tcpExecutorFixture(t)
	port := listener.Addr().(*net.TCPAddr).Port
	output, _, code, err := e.Execute(context.Background(), "telnet", []string{"127.0.0.1", strconv.Itoa(port)})
	if err != nil || code != 0 || !strings.Contains(output, "TCP port is open") {
		t.Fatalf("code=%d err=%v output=%s", code, err, output)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
func TestTCPProbeRefusalTimeoutCancellationAndInputValidation(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		e := tcpExecutorFixture(t)
		e.dial = func(context.Context, string, string) (net.Conn, error) { return nil, syscall.ECONNREFUSED }
		out, _, code, err := e.Execute(context.Background(), "telnet", []string{"127.0.0.1", "23"})
		if err == nil || code != 1 || !strings.Contains(out, "Connection refused") {
			t.Fatal(out, code, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		e := tcpExecutorFixture(t)
		e.probes["telnet"] = 20 * time.Millisecond
		e.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != "[::1]:23" {
				t.Error(network, address)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		out, _, code, err := e.Execute(context.Background(), "telnet", []string{"::1", "23"})
		if err == nil || code != 1 || !strings.Contains(out, "timed out") {
			t.Fatal(out, code, err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		e := tcpExecutorFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		e.dial = func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			cancel()
			<-ctx.Done()
			return nil, ctx.Err()
		}
		out, _, _, err := e.Execute(ctx, "telnet", []string{"127.0.0.1", "23"})
		if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(out, "canceled") {
			t.Fatal(out, err)
		}
	})
	t.Run("reject", func(t *testing.T) {
		e := tcpExecutorFixture(t)
		e.dial = func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("invalid arguments reached dial")
			return nil, nil
		}
		for _, args := range [][]string{{"example.com", "23"}, {"127.0.0.1;id", "23"}, {"127.0.0.1", "0"}, {"127.0.0.1", "65536"}, {"127.0.0.1", "23", "extra"}, {"::", "23"}, {"224.0.0.1", "23"}} {
			if _, _, _, err := e.Execute(context.Background(), "telnet", args); err == nil {
				t.Fatal("accepted", args)
			}
		}
	})
}

func TestTCPProbeCoreRejectsUnauthorizedAndInvalidRequestsBeforeDial(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewTCPProbeExecutor(cfg, &recordingExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	e.dial = func(context.Context, string, string) (net.Conn, error) { calls++; return nil, syscall.ECONNREFUSED }
	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	core := agent.NewCore(validation.New(cfg), policy.New(cfg), translator, e)
	for _, tc := range []struct{ role, host, stage string }{{"viewer", "127.0.0.1", "policy"}, {"netops", "example.com", "validation"}} {
		request, _ := contract.NewExecutionRequest("telnet", map[string]string{"host": tc.host}, contract.Metadata{BrokerType: "ssh", UserID: "alice", ClaimedRoles: []string{tc.role}})
		response := core.HandleRequest(context.Background(), request)
		if response.Result != contract.ResultRejected || response.FailureStage != tc.stage || calls != 0 {
			t.Fatal(response, calls)
		}
	}
	request, _ := contract.NewExecutionRequest("telnet", map[string]string{"host": "127.0.0.1"}, contract.Metadata{BrokerType: "ssh", UserID: "alice", ClaimedRoles: []string{"netops"}})
	response := core.HandleRequest(context.Background(), request)
	if response.Result != contract.ResultExecutionError || response.FailureStage != "execution" || calls != 1 {
		t.Fatal(response, calls)
	}
}
