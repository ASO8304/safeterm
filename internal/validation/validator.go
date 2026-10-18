// Package validation implements the real Validator (see internal/agent.Validator)
// that checks an incoming ExecutionRequest's arguments against the
// OperationDefinition/ParameterDefinition loaded from configuration (M3).
//
// It replaces the M2 PassThroughValidator placeholder. See
// docs/requirements/REQ-004 and docs/decisions/ADR-005 for the reasoning
// behind its two central security decisions: a whitelist of accepted
// argument names per operation, and a defense-in-depth denylist of shell
// metacharacters applied to every string value regardless of that
// parameter's own Pattern.
package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"safeterm/internal/config"
	"safeterm/internal/contract"
)

// dangerousChars matches characters commonly used for shell/command
// injection (statement separators, pipes, substitution, redirection,
// quoting). It is applied to every string argument unconditionally —
// even if the parameter's own Pattern would already reject it — so a
// mistake in one operation's Pattern can never become an injection
// vulnerability on its own (REQ-004, AC-7).
var dangerousChars = regexp.MustCompile("[;&|`$(){}<>\\n\\r'\"]")

// Validator checks ExecutionRequests against operation definitions loaded
// from configuration. It implements agent.Validator structurally (Go
// interfaces are satisfied implicitly; see internal/agent/core.go).
type Validator struct {
	operations map[string]config.OperationDefinition
}

// New builds a Validator from an already-loaded, already-validated Config
// (see config.LoadConfig, M3). It does not re-validate the config itself.
func New(cfg *config.Config) *Validator {
	ops := make(map[string]config.OperationDefinition, len(cfg.Operations))
	for _, op := range cfg.Operations {
		ops[op.Name] = op
	}
	return &Validator{operations: ops}
}

// Validate implements agent.Validator.
func (v *Validator) Validate(req *contract.ExecutionRequest) error {
	op, ok := v.operations[req.Command]
	if !ok {
		return fmt.Errorf("validation: unknown operation %q", req.Command)
	}

	defined := make(map[string]config.ParameterDefinition, len(op.Parameters))
	for _, p := range op.Parameters {
		defined[p.Name] = p
	}

	// Whitelist check (REQ-004, AC-6): any argument not explicitly defined
	// by this operation is rejected outright, regardless of its value.
	for name := range req.Arguments {
		if _, ok := defined[name]; !ok {
			return fmt.Errorf("validation: operation %q does not accept argument %q", op.Name, name)
		}
	}

	if len(op.RequiredAny) > 0 {
		present := false
		for _, name := range op.RequiredAny {
			if req.Arguments[name] != "" {
				present = true
			}
			for _, p := range op.Parameters {
				if p.Name == name && p.Default != "" {
					present = true
				}
			}
		}
		if !present {
			return fmt.Errorf("validation: operation %q: %s must be specified", op.Name, strings.Join(op.RequiredAny, " or "))
		}
	}

	for _, p := range op.Parameters {
		value, present := req.Arguments[p.Name]

		if !present {
			if p.Required {
				return fmt.Errorf("validation: operation %q: missing required argument %q", op.Name, p.Name)
			}
			continue
		}

		if dangerousChars.MatchString(value) {
			return fmt.Errorf("validation: operation %q: argument %q contains forbidden characters", op.Name, p.Name)
		}

		if err := validateTypedValue(op.Name, p, value); err != nil {
			return err
		}
	}

	return nil
}

func validateTypedValue(opName string, p config.ParameterDefinition, value string) error {
	switch p.Type {
	case config.ParamTypeInt:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("validation: operation %q: argument %q must be an integer, got %q", opName, p.Name, value)
		}
		if p.Min != nil && n < *p.Min {
			return fmt.Errorf("validation: operation %q: argument %q (%d) is below minimum %d", opName, p.Name, n, *p.Min)
		}
		if p.Max != nil && n > *p.Max {
			return fmt.Errorf("validation: operation %q: argument %q (%d) exceeds maximum %d", opName, p.Name, n, *p.Max)
		}
		return nil

	case config.ParamTypeString:
		if len(p.ValueMap) != 0 {
			if _, ok := p.ValueMap[value]; !ok {
				return fmt.Errorf("validation: operation %q: argument %q is not an allowed value", opName, p.Name)
			}
		}
		if p.Pattern == "" {
			return nil
		}
		matched, err := matchesPatternFully(p.Pattern, value)
		if err != nil {
			// A malformed regex is a configuration authoring bug (should
			// have been caught by ValidateConfig in M3), not a user error.
			// Validate defends against it anyway rather than assuming M3
			// was always run first.
			return fmt.Errorf("validation: operation %q: parameter %q has an invalid pattern: %w", opName, p.Name, err)
		}
		if !matched {
			return fmt.Errorf("validation: operation %q: argument %q does not match the required format", opName, p.Name)
		}
		return nil

	default:
		return fmt.Errorf("validation: operation %q: parameter %q has unsupported type %q", opName, p.Name, p.Type)
	}
}

// matchesPatternFully reports whether value matches pattern across its
// ENTIRE length, not just some substring of it. This matters because
// regexp.MatchString only checks whether the pattern occurs anywhere in
// the string; a config author who forgets to anchor their pattern with
// ^...$ (e.g. an IPv4 pattern) would otherwise let a value like
// "1.2.3.4; rm -rf /" pass, since "1.2.3.4" alone satisfies the pattern.
// Enforcing full-match here means every Pattern in configuration behaves
// as if it were anchored, regardless of how it was written.
func matchesPatternFully(pattern, value string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	loc := re.FindStringIndex(value)
	if loc == nil {
		return false, nil
	}
	return loc[0] == 0 && loc[1] == len(value), nil
}
