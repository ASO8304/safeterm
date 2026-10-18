package broker

import (
	"context"
	"errors"
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

var errShellEscape = errors.New("emergency session exit")

type GatewayClient interface {
	Handle(context.Context, *contract.ExecutionRequest) (*contract.ExecutionResponse, error)
	StartStream(context.Context, *contract.ExecutionRequest) (*isolation.StreamSession, error)
	Terminate(context.Context, contract.ControlRequest) (contract.ControlResult, error)
}

type Shell struct {
	editing       *configEditSession
	consoleLogin  func(context.Context, string, []byte) (Principal, time.Time, error)
	consoleLogout func(context.Context, Principal) error
	client        GatewayClient
	catalog       *Catalog
	principal     Principal
	input         io.Reader
	output        io.Writer
	configs       *ConfigBroker
	skipLF        bool
	history       []string
}

func NewAuthenticatedConsoleShell(client GatewayClient, catalog *Catalog, input io.Reader, output io.Writer, configs *ConfigBroker, login func(context.Context, string, []byte) (Principal, time.Time, error), logout func(context.Context, Principal) error) (*Shell, error) {
	if client == nil || catalog == nil || input == nil || output == nil || login == nil || logout == nil {
		return nil, fmt.Errorf("broker: invalid authenticated Console")
	}
	return &Shell{client: client, catalog: catalog, input: input, output: output, configs: configs, consoleLogin: login, consoleLogout: logout}, nil
}

func NewShell(client GatewayClient, catalog *Catalog, principal Principal, input io.Reader, output io.Writer) (*Shell, error) {
	return NewShellWithConfig(client, catalog, principal, input, output, nil)
}

func NewShellWithConfig(client GatewayClient, catalog *Catalog, principal Principal, input io.Reader, output io.Writer, configs *ConfigBroker) (*Shell, error) {
	return NewShellForBroker(client, catalog, principal, input, output, configs, "ssh")
}

func NewShellForBroker(client GatewayClient, catalog *Catalog, principal Principal, input io.Reader, output io.Writer, configs *ConfigBroker, brokerType string) (*Shell, error) {
	if client == nil || catalog == nil || !identityPattern.MatchString(principal.UserID) || len(principal.Roles) == 0 || input == nil || output == nil {
		return nil, fmt.Errorf("broker: invalid restricted shell configuration")
	}
	if brokerType != "ssh" && brokerType != "console" {
		return nil, fmt.Errorf("broker: invalid restricted shell type")
	}
	principal.BrokerType = brokerType
	return &Shell{client: client, catalog: catalog, principal: principal, input: input, output: output, configs: configs}, nil
}

func (s *Shell) Run(ctx context.Context) (runErr error) {
	if ctx == nil {
		return fmt.Errorf("broker: shell context is required")
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	defer func() { s.editing = nil }()
	defer func() {
		if errors.Is(context.Cause(ctx), errShellEscape) {
			fmt.Fprint(s.output, "\r\nsession closed (Ctrl+])\r\n")
			runErr = nil
		}
	}()
	input := make(chan byte, 2*maxShellLineBytes)
	done := make(chan struct{})
	defer close(done)
	go readBytes(s.input, input, done, cancel)
	if s.consoleLogin != nil {
		fmt.Fprint(s.output, "\r\nSafeTerm Console login (Ctrl+] exits)\r\n")
		authenticated := false
		for attempt := 0; attempt < 3; attempt++ {
			user, err := s.readLine(ctx, input, "Username: ", false, false)
			if err != nil {
				return err
			}
			password, err := s.readLine(ctx, input, "Password: ", true, false)
			if err != nil {
				return err
			}
			secret := []byte(password)
			principal, expires, err := s.consoleLogin(ctx, user, secret)
			clear(secret)
			password = ""
			if err != nil {
				fmt.Fprint(s.output, "Login failed.\r\n")
				continue
			}
			if principal.ConsoleSession == "" || principal.BrokerType != "console" || !identityPattern.MatchString(principal.UserID) || len(principal.Roles) == 0 || !expires.After(time.Now()) {
				return fmt.Errorf("broker: invalid Console session")
			}
			s.principal = principal
			var expire context.CancelFunc
			ctx, expire = context.WithDeadline(ctx, expires)
			defer expire()
			defer func() {
				cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				_ = s.consoleLogout(cleanup, principal)
			}()
			authenticated = true
			break
		}
		if !authenticated {
			return fmt.Errorf("broker: Console login denied")
		}
	}
	fmt.Fprintf(s.output, "\r\nSafeTerm restricted shell\r\nAuthenticated as %s.\r\nQuick start: '?' lists commands; 'execute ?' lists operations; 'config ?' lists files. '?' + Enter shows contextual help. Ctrl+] exits the session.\r\n", s.principal.UserID)
	for {
		line, err := s.readLine(ctx, input, s.prompt(), false, true)
		if err != nil {
			if err == io.EOF {
				fmt.Fprint(s.output, "\r\n")
				return nil
			}
			return err
		}
		exit, err := s.executeLine(ctx, strings.TrimSpace(line), input)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			fmt.Fprintf(s.output, "error: %v\r\n", err)
		}
		if exit {
			fmt.Fprint(s.output, "bye\r\n")
			return nil
		}
	}
}

func readBytes(reader io.Reader, output chan<- byte, done <-chan struct{}, cancel context.CancelCauseFunc) {
	defer close(output)
	buffer := make([]byte, 1)
	for {
		n, err := reader.Read(buffer)
		if n == 1 {
			if buffer[0] == 0x1d { // Ctrl+]: bypass command/editor input entirely.
				cancel(errShellEscape)
				return
			}
			select {
			case output <- buffer[0]:
			case <-done:
				return
			default:
				// Never block the emergency reader behind queued command input.
				cancel(fmt.Errorf("broker: shell input queue exceeded"))
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Shell) executeLine(ctx context.Context, line string, input <-chan byte) (bool, error) {
	if line == "" {
		return false, nil
	}
	if s.editing != nil {
		return s.configEditor(ctx, line, input)
	}
	fields := strings.Fields(line)
	if fields[len(fields)-1] == "?" {
		return false, s.questionHelp(ctx, fields[:len(fields)-1])
	}
	switch fields[0] {
	case "help":
		if len(fields) > 2 {
			return false, fmt.Errorf("usage: help [execute|config|keys|OPERATION]")
		}
		topic := ""
		if len(fields) == 2 {
			topic = fields[1]
		}
		return false, s.writeHelp(topic)
	case "execute":
		operation, args, ok := s.resolveOperation(fields[1:])
		if !ok {
			return false, s.operationTreeHelp(fields[1:])
		}
		arguments, err := parseOperationArguments(operation, args)
		if err != nil {
			return false, err
		}
		return false, s.runOperation(ctx, operation, arguments, input)
	case "show":
		return false, s.operationTreeHelp(fields[1:])
	case "config":
		if len(fields) == 1 {
			return false, s.listConfig(ctx)
		}
		if len(fields) != 2 {
			return false, fmt.Errorf("usage: config RESOURCE")
		}
		return false, s.enterConfig(ctx, fields[1])
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

// questionHelp is read-only and never dispatches an execution or mutation.
// Tab completion remains the editor's existing Linux-style workflow.
func (s *Shell) questionHelp(ctx context.Context, fields []string) error {
	row := func(name, description string) {
		s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s\n", name, strings.Join(strings.Fields(description), " "))))
	}
	if len(fields) == 0 {
		for _, item := range [][2]string{
			{"execute", "Execute an authorized command; use execute ?"},
			{"config", "Select a configuration file; use config ?"},
			{"show", "Show available operations; show COMMAND gives details"},
			{"whoami", "Show your identity and roles"},
			{"clear", "Clear the screen"},
			{"exit", "Close this session"},
		} {
			row(item[0], item[1])
		}
		return nil
	}
	switch fields[0] {
	case "execute":
		return s.operationTreeHelp(fields[1:])
	case "show":
		return s.operationTreeHelp(fields[1:])
	case "config":
		if len(fields) == 1 {
			return s.listConfig(ctx)
		}
	case "help":
		return s.writeHelp("")
	}
	return fmt.Errorf("no contextual help here; use ?, execute ?, or config ?")
}

func (s *Shell) listConfig(ctx context.Context) error {
	if s.configs == nil {
		return fmt.Errorf("configuration management is unavailable")
	}
	items, err := s.configs.List(ctx, s.principal)
	if err != nil {
		return err
	}
	for _, item := range items {
		s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s\n", item.ID, item.Description)))
	}
	return nil
}

func (s *Shell) readConfigValue(ctx context.Context, input <-chan byte, prompt string, secret bool) (string, error) {
	return s.readLine(ctx, input, prompt, secret, false)
}

func (s *Shell) describe(operation config.OperationDefinition) {
	fmt.Fprintf(s.output, "%s: %s\r\nprofile: %s\r\nmode: %s\r\n", operation.Name, operation.Description, operation.RunnerProfile, map[bool]string{true: "stream", false: "bounded"}[operation.LongRunning])
	if len(operation.Parameters) == 0 {
		fmt.Fprintf(s.output, "parameters: none\r\nexample: %s\r\n", operationExample(operation))
		return
	}
	if len(operation.RequiredAny) > 0 {
		fmt.Fprintf(s.output, "required: %s must be specified\r\n", strings.Join(operation.RequiredAny, " or "))
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
		if parameter.Default != "" {
			constraints += " default=" + parameter.Default
		}
		if len(parameter.ValueMap) != 0 {
			aliases := make([]string, 0, len(parameter.ValueMap))
			for alias := range parameter.ValueMap {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			constraints += " allowed=" + strings.Join(aliases, "|")
		}
		fmt.Fprintf(s.output, "- %s (%s, %s)%s\r\n", parameter.Name, parameter.Type, required, constraints)
	}
	fmt.Fprintf(s.output, "example: %s\r\n", operationExample(operation))
}

func parseOperationArguments(operation config.OperationDefinition, fields []string) (map[string]string, error) {
	arguments := make(map[string]string, len(fields))
	canonical := func(name string) string {
		for _, parameter := range operation.Parameters {
			if parameter.Name == name {
				return name
			}
			for _, alias := range parameter.Aliases {
				if alias == name {
					return parameter.Name
				}
			}
		}
		return name
	}
	positional := false
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if canonical(field) != field && !strings.Contains(field, "=") {
			if index+1 >= len(fields) || strings.Contains(fields[index+1], "=") {
				return nil, fmt.Errorf("option %q requires a value", field)
			}
			arguments[canonical(field)] = fields[index+1]
			index++
			continue
		}
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			positional = true
			continue
		}
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("argument %q must use KEY=VALUE syntax", field)
		}
		arguments[canonical(parts[0])] = parts[1]
	}
	if positional {
		if len(arguments) != 0 {
			return nil, fmt.Errorf("use either positional values or KEY=VALUE arguments, not both")
		}
		if len(fields) > len(operation.Parameters) {
			return nil, fmt.Errorf("too many values; use 'describe %s' for the expected parameters", operation.Name)
		}
		for index, value := range fields {
			if strings.Contains(value, "=") {
				return nil, fmt.Errorf("use either positional values or KEY=VALUE arguments, not both")
			}
			arguments[operation.Parameters[index].Name] = value
		}
	}
	return arguments, nil
}

func (s *Shell) writeHelp(topic string) error {
	switch topic {
	case "":
		fmt.Fprint(s.output, "Commands\r\n  execute ?       List authorized operations\r\n  show COMMAND    Show parameters and examples\r\n  config ?        List registered configuration files\r\n  config RESOURCE Enter the configuration editor\r\n  whoami          Show identity and roles\r\n  exit            Close session\r\n\r\nTab completes; ? then Enter explains the current choices. Ctrl+C stops an operation; Ctrl+] closes the session.\r\n")
	case "config":
		fmt.Fprint(s.output, "config RESOURCE -> edit SECTION -> set KEY VALUE -> diff -> end\r\nUse next to select another section, cancel to discard. Only end saves.\r\nFor secrets: set KEY prompts privately. Use - for the root section.\r\n")
	case "execute":
		return s.operationTreeHelp(nil)
	case "keys":
		fmt.Fprint(s.output, "Tab: complete; ? then Enter: contextual help; arrows: cursor/history; Ctrl+C: cancel input or stop stream; Ctrl+]: close session.\r\n")
	default:
		if op, _, ok := s.resolveOperation(strings.Fields(topic)); ok {
			s.describe(op)
			return nil
		}
		return fmt.Errorf("unknown help topic")
	}
	return nil
}

func operationExample(operation config.OperationDefinition) string {
	parts := append([]string{"execute"}, operationWords(operation.Name)...)
	for _, parameter := range operation.Parameters {
		value := "VALUE"
		if len(parameter.ValueMap) != 0 {
			aliases := make([]string, 0, len(parameter.ValueMap))
			for alias := range parameter.ValueMap {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			value = aliases[0]
		}
		parts = append(parts, parameter.Name+"="+value)
	}
	return strings.Join(parts, " ")
}

func (s *Shell) request(operation string, arguments map[string]string) (*contract.ExecutionRequest, error) {
	return contract.NewExecutionRequest(operation, arguments, contract.Metadata{
		ConsoleSession: s.principal.ConsoleSession,
		BrokerType:     s.principal.BrokerType, UserID: s.principal.UserID,
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
	fmt.Fprintf(s.output, "session %s started; Ctrl+C stops the operation; Ctrl+] exits the session\r\n", session.ID)
	terminating := false
	for {
		select {
		case <-ctx.Done():
			if !terminating {
				s.terminate(session.ID)
			}
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
	return s.client.Terminate(ctx, terminate(sessionID, contract.SessionOwner{BrokerType: s.principal.BrokerType, UserID: s.principal.UserID, ConsoleSession: s.principal.ConsoleSession}))
}

func (s *Shell) prompt() string {
	context := ""
	if e := s.editing; e != nil {
		context = "(" + e.resource
		if e.inSection {
			section := e.section
			if section == "" {
				section = "-"
			}
			context += "/" + section
		}
		context += ")"
	}
	return fmt.Sprintf("safeterm::%s%s> ", s.principal.UserID, context)
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
