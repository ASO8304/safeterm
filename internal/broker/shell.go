package broker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
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
	configs   *ConfigBroker
	skipLF    bool
	history   []string
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

func (s *Shell) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("broker: shell context is required")
	}
	input := make(chan byte, 64)
	done := make(chan struct{})
	defer close(done)
	go readBytes(s.input, input, done)
	fmt.Fprintf(s.output, "\r\nSafeTerm restricted shell\r\nAuthenticated as %s.\r\nQuick start: 'list' shows your operations; 'describe NAME' explains one; 'help' shows examples.\r\n", s.principal.UserID)
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

func (s *Shell) executeLine(ctx context.Context, line string, input <-chan byte) (bool, error) {
	if line == "" {
		return false, nil
	}
	fields := strings.Fields(line)
	switch fields[0] {
	case "help":
		if len(fields) > 2 {
			return false, fmt.Errorf("usage: help [run|config|keys|OPERATION]")
		}
		topic := ""
		if len(fields) == 2 {
			topic = fields[1]
		}
		return false, s.writeHelp(topic)
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
		arguments, err := parseOperationArguments(operation, fields[2:])
		if err != nil {
			return false, err
		}
		return false, s.runOperation(ctx, operation, arguments, input)
	case "config":
		return false, s.runConfig(ctx, fields[1:], input)
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

func (s *Shell) runConfig(ctx context.Context, fields []string, input <-chan byte) error {
	if s.configs == nil {
		return fmt.Errorf("configuration management is unavailable")
	}
	if len(fields) == 0 {
		return fmt.Errorf("usage: config list|show|describe|edit|apply|rollback")
	}
	switch fields[0] {
	case "list":
		if len(fields) != 1 {
			return fmt.Errorf("usage: config list")
		}
		items, err := s.configs.List(ctx, s.principal)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprint(s.output, "No authorized configuration resources.\r\n")
		}
		for _, item := range items {
			fmt.Fprintf(s.output, "%-24s [%s/%s] %s\r\n", item.ID, item.Format, item.Access, item.Description)
		}
	case "describe":
		if len(fields) != 2 {
			return fmt.Errorf("usage: config describe ID")
		}
		description, err := s.configs.Describe(ctx, s.principal, fields[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(s.output, "%s: %s\r\nprofile: %s format: %s access: %s\r\n", description.Resource.ID, description.Resource.Description, description.Resource.Profile, description.Resource.Format, description.Resource.Access)
		for _, field := range description.Fields {
			coordinate := field.Key
			if field.Section != "" {
				coordinate = field.Section + "." + field.Key
			}
			flags := []string{string(field.Type)}
			if field.Required {
				flags = append(flags, "required")
			}
			if field.Secret {
				flags = append(flags, "secret")
			}
			if field.ReadOnly {
				flags = append(flags, "read-only")
			}
			fmt.Fprintf(s.output, "- %-32s %s\r\n", coordinate, strings.Join(flags, ", "))
		}
	case "show":
		if len(fields) != 2 {
			return fmt.Errorf("usage: config show ID")
		}
		result, err := s.configs.Read(ctx, s.principal, fields[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(s.output, "resource: %s\r\nversion: %s\r\n", result.ResourceID, result.Version)
		for _, field := range result.Fields {
			coordinate := field.Key
			if field.Section != "" {
				coordinate = field.Section + "." + field.Key
			}
			fmt.Fprintf(s.output, "%-32s = %s\r\n", coordinate, field.Value)
		}
	case "edit":
		if len(fields) != 4 {
			return fmt.Errorf("usage: config edit ID SECTION KEY (use - for the root section)")
		}
		section := fields[2]
		if section == "-" {
			section = ""
		}
		read, err := s.configs.Read(ctx, s.principal, fields[1])
		if err != nil {
			return err
		}
		description, err := s.configs.Describe(ctx, s.principal, fields[1])
		if err != nil {
			return err
		}
		var schema *configmanager.FieldDescription
		for i := range description.Fields {
			if description.Fields[i].Section == section && description.Fields[i].Key == fields[3] {
				schema = &description.Fields[i]
				break
			}
		}
		if schema == nil {
			return fmt.Errorf("field is unknown or unauthorized")
		}
		if schema.ReadOnly {
			return fmt.Errorf("field is read-only")
		}
		value, err := s.readConfigValue(ctx, input, "new value: ", schema.Secret)
		if err != nil {
			return err
		}
		patch := configmanager.Patch{ResourceID: fields[1], Section: section, Key: fields[3], Value: value, BaseVersion: read.Version}
		validation, err := s.configs.ValidatePatch(ctx, s.principal, patch)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("validation failed: %s", validation.Message)
		}
		preview, err := s.configs.Preview(ctx, s.principal, patch)
		if err != nil {
			return err
		}
		fmt.Fprintf(s.output, "preview:\r\n%s\r\nproposal: %s\r\nexpires: %s\r\n", strings.ReplaceAll(preview.Diff, "\n", "\r\n"), preview.Token, preview.ExpiresAt.Format(time.RFC3339))
		if description.Resource.RequireSeparateApproval {
			fmt.Fprint(s.output, "Separate approver must run: config apply TOKEN\r\n")
			return nil
		}
		answer, err := s.readConfigValue(ctx, input, "apply this change? [y/N]: ", false)
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Fprint(s.output, "proposal not applied\r\n")
			return nil
		}
		result, err := s.configs.Apply(ctx, s.principal, preview.Token)
		if err != nil {
			return err
		}
		fmt.Fprintf(s.output, "applied: %s -> %s\r\n", result.PreviousVersion, result.Version)
	case "apply":
		if len(fields) != 2 {
			return fmt.Errorf("usage: config apply TOKEN")
		}
		result, err := s.configs.Apply(ctx, s.principal, fields[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(s.output, "applied %s: %s -> %s\r\n", result.ResourceID, result.PreviousVersion, result.Version)
	case "rollback":
		if len(fields) != 2 {
			return fmt.Errorf("usage: config rollback ID")
		}
		answer, err := s.readConfigValue(ctx, input, "restore newest backup? [y/N]: ", false)
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			return nil
		}
		result, err := s.configs.Rollback(ctx, s.principal, fields[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(s.output, "rolled back %s: %s -> %s\r\n", result.ResourceID, result.PreviousVersion, result.Version)
	default:
		return fmt.Errorf("unknown config command %q", fields[0])
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
	positional := false
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			positional = true
			continue
		}
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("argument %q must use KEY=VALUE syntax", field)
		}
		if _, exists := arguments[parts[0]]; exists {
			return nil, fmt.Errorf("argument %q was supplied more than once", parts[0])
		}
		arguments[parts[0]] = parts[1]
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
		fmt.Fprint(s.output, "Getting started\r\n  1. list                         see what you may run\r\n  2. describe NAME                see parameters and an example\r\n  3. run NAME [VALUE ...]         run with simple positional values\r\n     run NAME [KEY=VALUE ...]     run with explicit named values\r\n\r\nConfiguration\r\n  config list                    list resources you may access\r\n  config describe ID             show editable fields and types\r\n  config show ID                 read current non-secret values\r\n  config edit ID SECTION KEY     validate, preview, and propose/apply a value\r\n  config apply TOKEN             apply an approved proposal\r\n  config rollback ID             restore the newest retained backup\r\n\r\nSession\r\n  whoami                         show your identity and roles\r\n  clear                          clear the screen\r\n  exit                           close this session\r\n\r\nTry: 'help run', 'help config', 'help keys', or 'help OPERATION'.\r\n")
	case "run":
		fmt.Fprint(s.output, "Run operations\r\n  list\r\n  describe ping\r\n  run ping host=192.168.45.140\r\n  run trace host=192.168.45.140 port=999\r\n  run log service_A\r\n  run status_safeterm_gateway\r\n  run restart_safeterm_gateway\r\n\r\nLong-running operations stream output. Press Ctrl+C once to request a safe terminate.\r\nArguments are values, never shell text; pipes, redirects, and command substitution are unavailable.\r\n")
	case "config":
		fmt.Fprint(s.output, "Configuration workflow\r\n  config list\r\n  config describe cats-options\r\n  config show cats-options\r\n  config edit cats-options cats Cat_Port\r\n\r\nThe editor validates the new value and shows a preview before applying it. Tab completes resource, section, and field names.\r\n")
	case "keys":
		fmt.Fprint(s.output, "Editing keys\r\n  Tab             complete commands, operations, parameters, and allowed values\r\n  Up / Down       browse command history\r\n  Left / Right    move the cursor\r\n  Home / End      jump to the beginning or end\r\n  Backspace/Del   delete around the cursor\r\n  Ctrl+C          cancel input or terminate the active stream\r\n")
	default:
		operation, ok := s.catalog.AuthorizedOperation(topic, s.principal.Roles)
		if !ok {
			return fmt.Errorf("help topic or authorized operation %q was not found", topic)
		}
		s.describe(operation)
	}
	return nil
}

func operationExample(operation config.OperationDefinition) string {
	parts := []string{"run", operation.Name}
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
		BrokerType: s.principal.BrokerType, UserID: s.principal.UserID,
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
	return s.client.Terminate(ctx, terminate(sessionID, contract.SessionOwner{BrokerType: s.principal.BrokerType, UserID: s.principal.UserID}))
}

func (s *Shell) prompt() string { return fmt.Sprintf("safeterm::%s> ", s.principal.UserID) }

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
