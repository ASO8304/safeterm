package execution

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"safeterm/internal/agent"
	"safeterm/internal/config"
)

type portProbe struct {
	path    string
	udp     bool
	timeout int
}

// PortProbeExecutor builds fixed outbound-only nc argv from signed probe
// definitions. Its delegate must retain the executable integrity boundary.
// Operators cannot supply flags, ranges, stdin, listen or proxy modes.
type PortProbeExecutor struct {
	delegate agent.Executor
	probes   map[string]portProbe
}

func NewPortProbeExecutor(cfg *config.Config, delegate agent.Executor) (*PortProbeExecutor, error) {
	if delegate == nil {
		return nil, fmt.Errorf("execution: port probe requires an integrity-verified process delegate")
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	if err := config.ValidateRuntimeProbes(cfg); err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, executable := range cfg.Executables {
		paths[executable.ID] = executable.Path
	}
	e := &PortProbeExecutor{delegate: delegate, probes: map[string]portProbe{}}
	for _, op := range cfg.Operations {
		if !config.IsPortProbe(op.ExecutorKind) {
			continue
		}
		probe := portProbe{path: paths[config.ProbeExecutableRef(op)], udp: op.ExecutorKind == config.ExecutorUDPProbe}
		if probe.path == "" {
			return nil, fmt.Errorf("execution: probe %q requires its approved tool; complete the release update before starting runners", op.Name)
		}
		if probe.udp {
			probe.timeout = op.UDPProbe.TimeoutSeconds
		} else {
			probe.timeout = op.TCPProbe.TimeoutSeconds
		}
		e.probes[op.Name] = probe
	}
	return e, nil
}

func (e *PortProbeExecutor) Execute(ctx context.Context, command string, args []string) (string, string, int, error) {
	if e == nil || e.delegate == nil || ctx == nil {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: invalid port probe executor")
	}
	probe, known := e.probes[command]
	if !known {
		return e.delegate.Execute(ctx, command, args)
	}
	if len(args) != 2 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: port probe requires IP and port")
	}
	ip, err := netip.ParseAddr(args[0])
	if err != nil || ip.Zone() != "" || ip.Unmap().IsUnspecified() || ip.Unmap().IsMulticast() {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: port probe requires a literal unicast IP")
	}
	port, err := strconv.Atoi(args[1])
	if err != nil || port < 1 || port > 65535 {
		return "", "", ExitCodeUnavailable, fmt.Errorf("execution: port probe port must be 1..65535")
	}
	if err := ctx.Err(); err != nil {
		return "", "", ExitCodeUnavailable, err
	}
	host, portText := ip.Unmap().String(), strconv.Itoa(port)
	endpoint := net.JoinHostPort(host, portText)
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
	argv := []string{"-n", "-v", "-z", "-d", "-w", strconv.Itoa(probe.timeout)}
	if probe.udp {
		argv = append(argv, "-u")
	}
	argv = append(argv, "--", host, portText)
	attempt, cancel := context.WithTimeout(ctx, time.Duration(probe.timeout)*time.Second)
	defer cancel()
	// Capture bounded nc diagnostics privately and emit only the
	// protocol-aware summary. The lifecycle sink preserves audited process-group
	// cancellation/escalation.
	child := context.WithValue(attempt, streamSinkKey{}, nil)
	stdout, stderr, code, runErr := e.delegate.Execute(child, probe.path, argv)
	message := ""
	var resultErr error
	resultCode := code
	switch {
	case ctx.Err() != nil:
		message = "Connection test canceled."
		resultCode, resultErr = ExitCodeUnavailable, ctx.Err()
	case errors.Is(runErr, context.DeadlineExceeded) || attempt.Err() == context.DeadlineExceeded:
		message = "Connection timed out."
		resultCode, resultErr = 1, context.DeadlineExceeded
		if probe.udp {
			message = "UDP probe timed out. Port state is inconclusive; no response was verified."
			resultCode = 2
		}
	case runErr == nil && code == 0:
		message = "Connected to " + endpoint + ". TCP port is open."
		if probe.udp {
			message = "UDP probe sent to " + endpoint + ". Port state is inconclusive; nc -zu does not verify a response."
			resultCode, resultErr = 2, errors.New("UDP port state inconclusive")
		}
	default:
		message = "Connection test failed."
		detail := strings.ToLower(stdout + "\n" + stderr)
		switch {
		case strings.Contains(detail, "refused"):
			message = "Connection refused."
		case strings.Contains(detail, "unreachable") || strings.Contains(detail, "no route to host"):
			message = "Network or host unreachable."
		case strings.Contains(detail, "timed out"):
			message = "Connection timed out."
		}
		if probe.udp {
			message = "UDP scan failed; no port state was established."
		}
		resultErr = runErr
		if resultErr == nil {
			resultErr = fmt.Errorf("probe exited with code %d", code)
		}
	}
	if err := emit(message + "\n"); err != nil {
		return output, "", ExitCodeUnavailable, err
	}
	if resultErr != nil {
		return output, "", resultCode, fmt.Errorf("execution: port probe: %s: %w", message, resultErr)
	}
	return output, "", 0, nil
}
