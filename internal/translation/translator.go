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
	literal      string
	parameter    string
	valueMap     map[string]string
	defaultValue string
}

type argumentGroup struct {
	when string
	args []argument
}

type operation struct {
	groups      []argumentGroup
	separator   []string
	requiredAny []string
	defaults    map[string]string
	command     string
	args        []argument
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
		if op.ExecutorKind == config.ExecutorTCPProbe {
			ops[op.Name] = operation{command: op.Name, args: []argument{{parameter: "host"}, {parameter: "port", defaultValue: op.Parameters[1].Default}}}
			continue
		}
		if op.ExecutorKind == config.ExecutorSystemd {
			ops[op.Name] = operation{command: op.Name, args: []argument{{literal: op.Systemd.Action}}}
			continue
		}

		args, err := compileArguments(op.Name, op.ArgsTemplate, defined)
		if err != nil {
			return nil, err
		}
		compiled := operation{defaults: map[string]string{}, command: executablePaths[op.ExecutableRef], args: args, separator: append([]string(nil), op.ArgumentGroupSeparator...), requiredAny: append([]string(nil), op.RequiredAny...)}
		for _, p := range op.Parameters {
			if p.Default != "" {
				compiled.defaults[p.Name] = p.Default
			}
		}
		for _, group := range op.ArgumentGroups {
			args, err := compileArguments(op.Name, group.ArgsTemplate, defined)
			if err != nil {
				return nil, err
			}
			compiled.groups = append(compiled.groups, argumentGroup{when: group.When, args: args})
		}
		ops[op.Name] = compiled

	}
	return &Translator{operations: ops}, nil
}

func compileArguments(name string, tokens []string, defined map[string]config.ParameterDefinition) ([]argument, error) {
	args := make([]argument, len(tokens))
	for i, token := range tokens {
		if !strings.ContainsAny(token, "{}") {
			args[i].literal = token
			continue
		}
		if len(token) < 3 || token[0] != '{' || token[len(token)-1] != '}' || strings.ContainsAny(token[1:len(token)-1], "{}") {
			return nil, fmt.Errorf("translation: operation %q: args_template[%d]: invalid placeholder %q; expected a whole {name} token", name, i, token)
		}
		parameterName := token[1 : len(token)-1]
		parameter, exists := defined[parameterName]
		if !exists {
			return nil, fmt.Errorf("translation: operation %q: args_template[%d]: undefined parameter %q", name, i, parameterName)
		}
		args[i].parameter = parameterName
		args[i].valueMap = cloneMap(parameter.ValueMap)
		args[i].defaultValue = parameter.Default
	}
	return args, nil
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

	normalized := *req
	normalized.Arguments = cloneMap(req.Arguments)
	if normalized.Arguments == nil {
		normalized.Arguments = map[string]string{}
	}
	for name, value := range op.defaults {
		if _, exists := normalized.Arguments[name]; !exists {
			normalized.Arguments[name] = value
		}
	}
	req = &normalized

	if len(op.requiredAny) > 0 {
		found := false
		for _, name := range op.requiredAny {
			if req.Arguments[name] != "" {
				found = true
			}
		}
		if !found {
			return "", nil, fmt.Errorf("translation: %s must be specified", strings.Join(op.requiredAny, " or "))
		}
	}
	args, err := renderArguments(req.Command, op.args, req)
	if err != nil {
		return "", nil, err
	}
	groups := 0
	for _, group := range op.groups {
		if req.Arguments[group.when] == "" {
			continue
		}
		extra, err := renderArguments(req.Command, group.args, req)
		if err != nil {
			return "", nil, err
		}
		if groups > 0 {
			args = append(args, op.separator...)
		}
		args = append(args, extra...)
		groups++
	}

	return op.command, args, nil
}

func renderArguments(command string, tokens []argument, req *contract.ExecutionRequest) ([]string, error) {
	args := make([]string, len(tokens))
	for i, token := range tokens {
		if token.parameter == "" {
			args[i] = token.literal
			continue
		}
		value, present := req.Arguments[token.parameter]
		if !present {
			if token.defaultValue == "" {
				return nil, fmt.Errorf("translation: operation %q: missing argument %q required by template", command, token.parameter)
			}
			value = token.defaultValue
		}
		if len(token.valueMap) != 0 {
			mapped, allowed := token.valueMap[value]
			if !allowed {
				return nil, fmt.Errorf("translation: operation %q: argument %q is not mapped", command, token.parameter)
			}
			value = mapped
		}
		args[i] = value
	}
	return args, nil
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
