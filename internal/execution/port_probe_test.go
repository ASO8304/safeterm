package execution

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

type probeDelegate func(context.Context, string, []string) (string, string, int, error)

func (f probeDelegate) Execute(ctx context.Context, command string, args []string) (string, string, int, error) {
	return f(ctx, command, args)
}

func probeFixture(t *testing.T, delegate agent.Executor) (*PortProbeExecutor, *config.Config) {
	t.Helper()
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewPortProbeExecutor(cfg, delegate)
	if err != nil {
		t.Fatal(err)
	}
	return e, cfg
}

func TestPortProbeUsesOnlyFixedNetcatArguments(t *testing.T) {
	for _, tc := range []struct {
		name, ip, port, path string
		args                 []string
	}{
		{"telnet", "127.0.0.1", "443", "/usr/bin/nc.openbsd", []string{"-n", "-v", "-z", "-d", "-w", "5", "--", "127.0.0.1", "443"}},
		{"udp-test", "192.0.2.1", "53", "/usr/bin/nc.openbsd", []string{"-n", "-v", "-z", "-d", "-w", "5", "-u", "--", "192.0.2.1", "53"}},
		{"udp-test", "::1", "53", "/usr/bin/nc.openbsd", []string{"-n", "-v", "-z", "-d", "-w", "5", "-u", "--", "::1", "53"}},
	} {
		t.Run(tc.name+tc.ip, func(t *testing.T) {
			calls := 0
			e, _ := probeFixture(t, probeDelegate(func(ctx context.Context, path string, args []string) (string, string, int, error) {
				calls++
				if path != tc.path || !reflect.DeepEqual(args, tc.args) || streamSinkFrom(ctx) != nil {
					t.Fatal("unsafe or unexpected delegation", path, args)
				}
				return "", "untrusted raw diagnostic succeeded!", 0, nil
			}))
			out, stderr, code, err := e.Execute(context.Background(), tc.name, []string{tc.ip, tc.port})
			wantCode := 0
			if tc.name == "udp-test" {
				wantCode = 2
			}
			if code != wantCode || (err == nil) != (wantCode == 0) || calls != 1 || stderr != "" || strings.Contains(out, "untrusted") || (wantCode == 2 && !strings.Contains(out, "inconclusive")) {
				t.Fatal(out, stderr, code, err, calls)
			}
		})
	}
}

func TestNetcatTCPConnectsAndClosesWithoutApplicationData(t *testing.T) {
	if _, err := os.Stat("/usr/bin/nc.openbsd"); err != nil {
		t.Skip("netcat-openbsd is exercised by the disposable systemd gate")
	}
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
		buffer := make([]byte, 1)
		n, err := conn.Read(buffer)
		if n != 0 || err != io.EOF {
			done <- errors.New("netcat TCP probe sent application data or left the connection open")
			return
		}
		done <- nil
	}()
	runner, err := New(time.Second * 10)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := probeFixture(t, runner)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	out, _, code, err := e.Execute(context.Background(), "telnet", []string{"127.0.0.1", port})
	if err != nil || code != 0 || !strings.Contains(out, "TCP port is open") {
		t.Fatal(out, code, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPortProbeRefusalCancellationTimeoutAndInvalidInputs(t *testing.T) {
	e, _ := probeFixture(t, probeDelegate(func(context.Context, string, []string) (string, string, int, error) {
		return "", "nc: connect failed: Connection refused", 1, errors.New("exit status 1")
	}))
	out, _, code, err := e.Execute(context.Background(), "telnet", []string{"127.0.0.1", "23"})
	if err == nil || code != 1 || !strings.Contains(out, "Connection refused") {
		t.Fatal(out, code, err)
	}
	for _, operation := range []string{"telnet", "udp-test"} {
		t.Run(operation+"-cancel", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e, _ := probeFixture(t, probeDelegate(func(child context.Context, _ string, _ []string) (string, string, int, error) {
				cancel()
				<-child.Done()
				return "", "", -1, child.Err()
			}))
			out, _, code, err := e.Execute(ctx, operation, []string{"127.0.0.1", "53"})
			if !errors.Is(err, context.Canceled) || code != -1 || !strings.Contains(out, "canceled") {
				t.Fatal(out, code, err)
			}
		})
		t.Run(operation+"-timeout", func(t *testing.T) {
			e, _ := probeFixture(t, probeDelegate(func(context.Context, string, []string) (string, string, int, error) {
				return "", "", -1, context.DeadlineExceeded
			}))
			out, _, code, err := e.Execute(context.Background(), operation, []string{"127.0.0.1", "53"})
			if !errors.Is(err, context.DeadlineExceeded) || code == 0 || !strings.Contains(out, "timed out") || operation == "udp-test" && (code != 2 || !strings.Contains(out, "inconclusive")) {
				t.Fatal(out, code, err)
			}
		})
		e, _ := probeFixture(t, probeDelegate(func(context.Context, string, []string) (string, string, int, error) {
			t.Fatal("invalid input reached an executable")
			return "", "", 0, nil
		}))
		for _, args := range [][]string{{"example.com", "53"}, {"127.0.0.1;id", "53"}, {"127.0.0.1", "0"}, {"127.0.0.1", "1-65535"}, {"127.0.0.1", "65536"}, {"127.0.0.1", "53", "--script=all"}, {"::", "53"}, {"ff02::1", "53"}} {
			if _, _, _, err := e.Execute(context.Background(), operation, args); err == nil {
				t.Fatal("accepted", operation, args)
			}
		}
	}
}

func TestPortProbeCoreRejectsUnauthorizedAndInvalidRequestsBeforeExecution(t *testing.T) {
	calls := 0
	e, cfg := probeFixture(t, probeDelegate(func(context.Context, string, []string) (string, string, int, error) {
		calls++
		return "", "refused", 1, errors.New("exit 1")
	}))
	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	core := agent.NewCore(validation.New(cfg), policy.New(cfg), translator, e)
	for _, operation := range []string{"telnet", "udp-test"} {
		for _, tc := range []struct{ role, host, port, stage string }{{"viewer", "127.0.0.1", "53", "policy"}, {"netops", "example.com", "53", "validation"}, {"netops", "127.0.0.1", "1-65535", "validation"}} {
			req, _ := contract.NewExecutionRequest(operation, map[string]string{"host": tc.host, "port": tc.port}, contract.Metadata{BrokerType: "ssh", UserID: "alice", ClaimedRoles: []string{tc.role}})
			response := core.HandleRequest(context.Background(), req)
			if response.Result != contract.ResultRejected || response.FailureStage != tc.stage || calls != 0 {
				t.Fatal(response, calls)
			}
		}
	}
}
