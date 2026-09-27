package config

// ParameterType restricts what kind of value a parameter may hold.
// This only describes the SHAPE of a parameter; actually checking a real
// user-supplied value against these rules is the job of the Validation
// Engine (M4), not this package.
type ParameterType string

const (
	ParamTypeInt    ParameterType = "int"
	ParamTypeString ParameterType = "string"
)

// ParameterDefinition describes one argument an Operation accepts.
// Min/Max only apply when Type is ParamTypeInt; Pattern only applies
// when Type is ParamTypeString. ValidateConfig enforces this so an
// inconsistent definition (e.g. a string parameter with a numeric range)
// is rejected at load time, not silently ignored at runtime.
type ParameterDefinition struct {
	Name     string        `json:"name"`
	Type     ParameterType `json:"type"`
	Required bool          `json:"required"`
	Min      *int          `json:"min,omitempty"`
	Max      *int          `json:"max,omitempty"`
	Pattern  string        `json:"pattern,omitempty"`
}

// OperationDefinition describes one safe operation SafeTerm exposes to
// users, and how it will eventually map to a real system command
// (Translation Layer, M6). ArgsTemplate is not interpreted by this
// package at all — it is inert data until M6 exists.
type OperationDefinition struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Command      string                `json:"command"`
	ArgsTemplate []string              `json:"args_template"`
	Parameters   []ParameterDefinition `json:"parameters"`
	LongRunning  bool                  `json:"long_running"`
}

// RoleDefinition describes one role that can be granted to a user.
type RoleDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PolicyDefinition maps a role to the set of operation names it may
// invoke. Real enforcement is provided by internal/policy (M5);
// this package only guarantees the mapping itself is internally
// consistent (every referenced role and operation actually exists).
type PolicyDefinition struct {
	Role       string   `json:"role"`
	Operations []string `json:"operations"`
}

// Config is the fully loaded and structurally-validated configuration for
// a SafeTerm deployment.
type Config struct {
	Operations []OperationDefinition
	Roles      []RoleDefinition
	Policies   []PolicyDefinition
}
