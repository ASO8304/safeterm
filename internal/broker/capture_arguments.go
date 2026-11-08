package broker

import (
	"fmt"
	"strings"

	"safeterm/internal/config"
)

func captureKey(op config.OperationDefinition, key string) bool {
	for _, p := range op.Parameters {
		if p.Name == key {
			return true
		}
	}
	return false
}

// Canonical key/value pairs map to the same signed parameters and downstream
// validation as before. Legacy KEY=VALUE inputs remain accepted for scripts.
func parseCaptureArguments(op config.OperationDefinition, fields []string) (map[string]string, error) {
	args := map[string]string{}
	for i := 0; i < len(fields); i++ {
		key, value, assigned := strings.Cut(fields[i], "=")
		if !captureKey(op, key) {
			return nil, fmt.Errorf("capture: unknown filter %q; use capture host IP port NUMBER protocol PROTOCOL", key)
		}
		if !assigned {
			if i+1 >= len(fields) || captureKey(op, fields[i+1]) {
				return nil, fmt.Errorf("capture: %s requires a value", key)
			}
			i++
			value = fields[i]
		}
		if value == "" {
			return nil, fmt.Errorf("capture: %s requires a value", key)
		}
		if _, exists := args[key]; exists {
			return nil, fmt.Errorf("capture: %s may be specified only once", key)
		}
		args[key] = value
	}
	return args, nil
}

func captureCompletion(op config.OperationDefinition, completed []string, prefix string) []string {
	used := map[string]bool{}
	for i := 0; i < len(completed); i++ {
		key, _, assigned := strings.Cut(completed[i], "=")
		if !captureKey(op, key) {
			return nil
		}
		used[key] = true
		if !assigned {
			if i+1 == len(completed) {
				for _, p := range op.Parameters {
					if p.Name == key {
						var values []string
						for v := range p.ValueMap {
							values = append(values, v)
						}
						return matching(values, prefix)
					}
				}
				return nil
			}
			i++
		}
	}
	var values []string
	for _, p := range op.Parameters {
		if !used[p.Name] {
			values = append(values, p.Name)
		}
	}
	return matching(values, prefix)
}
