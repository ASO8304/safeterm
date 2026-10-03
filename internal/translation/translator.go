// Package translation maps validated, authorized requests to a configured
// executable and separate arguments. It never runs a shell or a process.
// See REQ-006 and ADR-007 for the intentionally limited template grammar.
package translation

import (
	"fmt"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/contract"
)

type argument struct {
	literal   string
	parameter string
	valueMap  map[string]string
}

type operation struct {
	command string
	args    []argument
}

// Translator implements agent.Translator. Its private template snapshot is
// read-only after construction; returned slices never alias that snapshot.
type Translator struct {
	operations map[string]operation
}

// New validates configuration and compiles whole-token placeholders. A
// malformed template fails before requests can be served by this stage.
func New(cfg *config.Config) (*Translator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("translation: nil configuration")
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("translation: invalid configuration: %w", err)
	}
	ops := make(map[string]operation, len(cfg.Operations))
	executablePaths := make(map[string]string, len(cfg.Executables))
	for _, executable := range cfg.Executables {
		executablePaths[executable.ID] = executable.Path
	}
	for _, op := range cfg.Operations {
		defined := make(map[string]config.ParameterDefinition, len(op.Parameters))
		for _, p := range op.Parameters {
			if _, exists := defined[p.Name]; exists {
				return nil, fmt.Errorf("translation: operation %q: duplicate parameter %q", op.Name, p.Name)
			}
			defined[p.Name] = p
		}
		if op.ExecutorKind == config.ExecutorSystemd {
			ops[op.Name] = operation{command: op.Systemd.Unit, args: []argument{{literal: op.Systemd.Action}}}
			continue
		}
		args := make([]argument, len(op.ArgsTemplate))
		for i, token := range op.ArgsTemplate {
			if !strings.ContainsAny(token, "{}") {
				args[i].literal = token
				continue
			}
			if len(token) < 3 || token[0] != '{' || token[len(token)-1] != '}' || strings.ContainsAny(token[1:len(token)-1], "{}") {
				return nil, fmt.Errorf("translation: operation %q: args_template[%d]: invalid placeholder %q; expected a whole {name} token", op.Name, i, token)
			}
			name := token[1 : len(token)-1]
			parameter, exists := defined[name]
			if !exists {
				return nil, fmt.Errorf("translation: operation %q: args_template[%d]: undefined parameter %q", op.Name, i, name)
			}
			args[i].parameter = name
			args[i].valueMap = cloneMap(parameter.ValueMap)
		}
		ops[op.Name] = operation{command: executablePaths[op.ExecutableRef], args: args}
	}
	return &Translator{operations: ops}, nil
}

// Translate requires a request that has passed Validation and Policy using
// the same configuration. It checks missing substitutions defensively, but
// does not replace either of those security stages. Every error returns no
// executable and no partial arguments.
func (t *Translator) Translate(req *contract.ExecutionRequest) (string, []string, error) {
	if req == nil {
		return "", nil, fmt.Errorf("translation: nil request")
	}
	op, ok := t.operations[req.Command]
	if !ok {
		return "", nil, fmt.Errorf("translation: unknown operation %q", req.Command)
	}
	args := make([]string, len(op.args))
	for i, token := range op.args {
		if token.parameter == "" {
			args[i] = token.literal
			continue
		}
		value, present := req.Arguments[token.parameter]
		if !present {
			return "", nil, fmt.Errorf("translation: operation %q: missing argument %q required by template", req.Command, token.parameter)
		}
		if len(token.valueMap) != 0 {
			mapped, allowed := token.valueMap[value]
			if !allowed {
				return "", nil, fmt.Errorf("translation: operation %q: argument %q is not mapped", req.Command, token.parameter)
			}
			value = mapped
		}
		args[i] = value
	}
	return op.command, args, nil
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
