package execution

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
)

// TCPProbeExecutor binds signed operation names to a single, bounded TCP
// handshake. It never sends application data, resolves DNS, invokes telnet or
// opens a shell. Other commands still cross the existing process integrity gate.
type TCPProbeExecutor struct {
	delegate agent.Executor
	probes   map[string]time.Duration
	dial     func(context.Context, string, string) (net.Conn, error)
}

func NewTCPProbeExecutor(cfg *config.Config, delegate agent.Executor) (*TCPProbeExecutor, error) {
	if delegate == nil {
		return nil, fmt.Errorf("execution: TCP probe requires a process delegate")
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	e := &TCPProbeExecutor{delegate: delegate, probes: map[string]time.Duration{}, dial: (&net.Dialer{}).DialContext}
	for _, op := range cfg.Operations {
		if op.ExecutorKind == config.ExecutorTCPProbe {
			e.probes[op.Name] = time.Duration(op.TCPProbe.TimeoutSeconds) * time.Second
		}
	}
	return e, nil
}
func (e *TCPProbeExecutor) Execute(ctx context.Context, command string, args []string) (string, string, int, error) {
	if e == nil || e.delegate == nil || ctx == nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: invalid TCP probe executor")
	}
	timeout, known := e.probes[command]
	if !known {
		return e.delegate.Execute(ctx, command, args)
	}
	if len(args) != 2 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: TCP probe requires IP and port")
	}
	ip, err := netip.ParseAddr(args[0])
	if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: invalid TCP probe IP")
	}
	port, err := strconv.Atoi(args[1])
	if err != nil || port < 1 || port > 65535 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: TCP probe port must be 1..65535")
	}
	if err := ctx.Err(); err != nil {
		return "", "", ExitCodeUnavailable, err
	}
	endpoint := net.JoinHostPort(ip.Unmap().String(), strconv.Itoa(port))
	output := ""
	emit := func(text string) error {
		output += text
		if sink := streamSinkFrom(ctx); sink != nil {
			return sink.Emit(StreamStdout, []byte(text))
		}
		return nil
	}
	if err := emit("Trying " + endpoint + "...\n"); err != nil {
		return output, "", ExitCodeUnavailable, err
	}
	attempt, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := e.dial(attempt, "tcp", endpoint)
	if conn != nil {
		_ = conn.Close()
	}
	if ctx.Err() != nil {
		_ = emit("Connection test canceled.\n")
		return output, "", ExitCodeUnavailable, fmt.Errorf("execution: TCP probe canceled: %w", ctx.Err())
	}
	if err != nil {
		message := "Connection failed."
		switch {
		case errors.Is(err, context.DeadlineExceeded) || attempt.Err() == context.DeadlineExceeded:
			message = "Connection timed out."
		case errors.Is(err, syscall.ECONNREFUSED):
			message = "Connection refused."
		case errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH):
			message = "Network or host unreachable."
		}
		if emitErr := emit(message + "\n"); emitErr != nil {
			return output, "", ExitCodeUnavailable, emitErr
		}
		return output, "", 1, fmt.Errorf("execution: TCP probe: %s", message)
	}
	if err := emit("Connected to " + endpoint + ". TCP port is open.\n"); err != nil {
		return output, "", ExitCodeUnavailable, err
	}
	return output, "", 0, nil
}
