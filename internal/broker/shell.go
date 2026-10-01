package broker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

const maxShellLineBytes = 4096

type GatewayClient interface {
	Handle(context.Context, *contract.ExecutionRequest) (*contract.ExecutionResponse, error)
	StartStream(context.Context, *contract.ExecutionRequest) (*isolation.StreamSession, error)
	Terminate(context.Context, contract.ControlRequest) (contract.ControlResult, error)
}

type Shell struct {
	client    GatewayClient
	catalog   *Catalog
	principal Principal
	input     io.Reader
	output    io.Writer
	skipLF    bool
}

func NewShell(client GatewayClient, catalog *Catalog, principal Principal, input io.Reader, output io.Writer) (*Shell, error) {
	if client == nil || catalog == nil || !identityPattern.MatchString(principal.UserID) || len(principal.Roles) == 0 || input == nil || output == nil {
		return nil, fmt.Errorf("broker: invalid restricted shell configuration")
	}
	return &Shell{client: client, catalog: catalog, principal: principal, input: input, output: output}, nil
}

func (s *Shell) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("broker: shell context is required")
	}
	input := make(chan byte, 64)
	done := make(chan struct{})
	defer close(done)
	go readBytes(s.input, input, done)
	fmt.Fprintf(s.output, "\r\nSafeTerm restricted shell\r\nAuthenticated as %s. Type 'help' for commands.\r\n", s.principal.UserID)
	for {
		fmt.Fprint(s.output, "safeterm> ")
		line, err := s.readLine(ctx, input)
		if err != nil {
			if err == io.EOF {
				fmt.Fprint(s.output, "\r\n")
				return nil
			}
			return err
		}
		exit, err := s.executeLine(ctx, strings.TrimSpace(line), input)
		if err != nil {
			fmt.Fprintf(s.output, "error: %v\r\n", err)
		}
		if exit {
			fmt.Fprint(s.output, "bye\r\n")
			return nil
		}
	}
}

func readBytes(reader io.Reader, output chan<- byte, done <-chan struct{}) {
	defer close(output)
	buffer := make([]byte, 1)
	for {
		n, err := reader.Read(buffer)
		if n == 1 {
			select {
			case output <- buffer[0]:
			case <-done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Shell) readLine(ctx context.Context, input <-chan byte) (string, error) {
	line := make([]byte, 0, 128)
	escapeState := 0
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case b, ok := <-input:
			if !ok {
				return "", io.EOF
			}
			if escapeState == 1 {
				if b == '[' {
					escapeState = 2
				} else {
					escapeState = 0
				}
				continue
			}
			if escapeState == 2 {
				if b >= 0x40 && b <= 0x7e {
					escapeState = 0
				}
				continue
			}
			if b != '\n' {
				s.skipLF = false
			}
			switch b {
			case 0x1b:
				escapeState = 1
			case 0x03:
				line = line[:0]
				fmt.Fprint(s.output, "^C\r\n")
				return "", nil
			case 0x04:
				if len(line) == 0 {
					return "", io.EOF
				}
			case '\r':
				s.skipLF = true
				fmt.Fprint(s.output, "\r\n")
				return string(line), nil
			case '\n':
				if s.skipLF {
					s.skipLF = false
					continue
				}
				fmt.Fprint(s.output, "\r\n")
				return string(line), nil
			case 0x7f, 0x08:
				if len(line) > 0 {
					line = line[:len(line)-1]
					fmt.Fprint(s.output, "\b \b")
				}
			default:
				if b >= 0x20 && b <= 0x7e {
					if len(line) >= maxShellLineBytes {
						fmt.Fprint(s.output, "\a")
						continue
					}
					line = append(line, b)
					_, _ = s.output.Write([]byte{b})
				}
			}
		}
	}
}

func (s *Shell) executeLine(ctx context.Context, line string, input <-chan byte) (bool, error) {
	if line == "" {
		return false, nil
	}
	fields := strings.Fields(line)
	switch fields[0] {
	case "help":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: help")
		}
		fmt.Fprint(s.output, "help                 show this help\r\nlist                 list authorized operations\r\ndescribe NAME        show operation parameters\r\nrun NAME [K=V ...]   run an authorized operation\r\nwhoami               show authenticated identity\r\nclear                clear the terminal\r\nexit                 close this session\r\n")
	case "list":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: list")
		}
		operations := s.catalog.AuthorizedOperations(s.principal.Roles)
		if len(operations) == 0 {
			fmt.Fprint(s.output, "No authorized operations.\r\n")
		}
		for _, operation := range operations {
			mode := "bounded"
			if operation.LongRunning {
				mode = "stream"
			}
			fmt.Fprintf(s.output, "%-24s [%s] %s\r\n", operation.Name, mode, operation.Description)
		}
	case "describe":
		if len(fields) != 2 {
			return false, fmt.Errorf("usage: describe NAME")
		}
		operation, ok := s.catalog.AuthorizedOperation(fields[1], s.principal.Roles)
		if !ok {
			return false, fmt.Errorf("operation %q is unknown or unauthorized", fields[1])
		}
		s.describe(operation)
	case "run":
		if len(fields) < 2 {
			return false, fmt.Errorf("usage: run NAME [KEY=VALUE ...]")
		}
		operation, ok := s.catalog.AuthorizedOperation(fields[1], s.principal.Roles)
		if !ok {
			return false, fmt.Errorf("operation %q is unknown or unauthorized", fields[1])
		}
		arguments, err := parseArguments(fields[2:])
		if err != nil {
			return false, err
		}
		return false, s.runOperation(ctx, operation, arguments, input)
	case "whoami":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: whoami")
		}
		fmt.Fprintf(s.output, "user: %s\r\nroles: %s\r\n", s.principal.UserID, strings.Join(s.principal.Roles, ", "))
	case "clear":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: clear")
		}
		fmt.Fprint(s.output, "\x1b[2J\x1b[H")
	case "exit", "logout", "quit":
		if len(fields) != 1 {
			return false, fmt.Errorf("usage: exit")
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown shell command %q; use 'help'", fields[0])
	}
	return false, nil
}

func (s *Shell) describe(operation config.OperationDefinition) {
	fmt.Fprintf(s.output, "%s: %s\r\nprofile: %s\r\nmode: %s\r\n", operation.Name, operation.Description, operation.RunnerProfile, map[bool]string{true: "stream", false: "bounded"}[operation.LongRunning])
	if len(operation.Parameters) == 0 {
		fmt.Fprint(s.output, "parameters: none\r\n")
		return
	}
	parameters := append([]config.ParameterDefinition(nil), operation.Parameters...)
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	for _, parameter := range parameters {
		required := "optional"
		if parameter.Required {
			required = "required"
		}
		constraints := ""
		if parameter.Min != nil {
			constraints += fmt.Sprintf(" min=%d", *parameter.Min)
		}
		if parameter.Max != nil {
			constraints += fmt.Sprintf(" max=%d", *parameter.Max)
		}
		if parameter.Pattern != "" {
			constraints += " pattern=" + parameter.Pattern
		}
		fmt.Fprintf(s.output, "- %s (%s, %s)%s\r\n", parameter.Name, parameter.Type, required, constraints)
	}
}

func parseArguments(fields []string) (map[string]string, error) {
	arguments := make(map[string]string, len(fields))
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("argument %q must use KEY=VALUE syntax", field)
		}
		if _, exists := arguments[parts[0]]; exists {
			return nil, fmt.Errorf("argument %q was supplied more than once", parts[0])
		}
		arguments[parts[0]] = parts[1]
	}
	return arguments, nil
}

func (s *Shell) request(operation string, arguments map[string]string) (*contract.ExecutionRequest, error) {
	return contract.NewExecutionRequest(operation, arguments, contract.Metadata{
		BrokerType: "ssh", UserID: s.principal.UserID,
		ClaimedRoles: append([]string(nil), s.principal.Roles...), ReceivedAt: time.Now().UTC(),
	})
}

func (s *Shell) runOperation(ctx context.Context, operation config.OperationDefinition, arguments map[string]string, input <-chan byte) error {
	request, err := s.request(operation.Name, arguments)
	if err != nil {
		return err
	}
	if !operation.LongRunning {
		response, err := s.client.Handle(ctx, request)
		if err != nil {
			return err
		}
		s.writeResponse(response)
		return nil
	}
	session, err := s.client.StartStream(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.output, "session %s started; press Ctrl+C to terminate\r\n", session.ID)
	terminating := false
	for {
		select {
		case <-ctx.Done():
			s.terminate(session.ID)
			return ctx.Err()
		case b, ok := <-input:
			if !ok {
				s.terminate(session.ID)
				return io.EOF
			}
			if b == 0x03 && !terminating {
				terminating = true
				result, err := s.terminate(session.ID)
				if err != nil {
					fmt.Fprintf(s.output, "\r\nterminate failed: %v\r\n", err)
				} else {
					fmt.Fprintf(s.output, "\r\nterminate: %s\r\n", result.Status)
				}
			}
		case frame, ok := <-session.Events:
			if !ok {
				return fmt.Errorf("stream ended without a final response")
			}
			switch frame.Type {
			case contract.FrameStdout, contract.FrameStderr:
				s.writeTerminalData(frame.Data)
			case contract.FrameSessionExited:
				fmt.Fprint(s.output, "\r\n")
				s.writeResponse(frame.Response)
				return nil
			}
		}
	}
}

func (s *Shell) terminate(sessionID string) (contract.ControlResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.Terminate(ctx, (SSHControlAdapter{}).Interrupt(sessionID, s.principal.UserID))
}

func (s *Shell) writeResponse(response *contract.ExecutionResponse) {
	if response == nil {
		fmt.Fprint(s.output, "result: invalid response\r\n")
		return
	}
	if response.Stdout != "" {
		s.writeTerminalData([]byte(response.Stdout))
		if !strings.HasSuffix(response.Stdout, "\n") {
			fmt.Fprint(s.output, "\r\n")
		}
	}
	if response.Stderr != "" {
		s.writeTerminalData([]byte(response.Stderr))
		if !strings.HasSuffix(response.Stderr, "\n") {
			fmt.Fprint(s.output, "\r\n")
		}
	}
	fmt.Fprintf(s.output, "result: %s exit=%d", response.Result, response.ExitCode)
	if response.Message != "" {
		fmt.Fprintf(s.output, " message=%s", response.Message)
	}
	fmt.Fprint(s.output, "\r\n")
}

func (s *Shell) writeTerminalData(data []byte) {
	text := strings.ToValidUTF8(string(data), "�")
	for _, character := range text {
		switch {
		case character == '\n':
			fmt.Fprint(s.output, "\r\n")
		case character == '\t':
			fmt.Fprint(s.output, "\t")
		case character < 0x20 || (character >= 0x7f && character <= 0x9f):
			fmt.Fprint(s.output, "�")
		default:
			fmt.Fprint(s.output, string(character))
		}
	}
}
