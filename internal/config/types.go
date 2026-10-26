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

// RunnerProfile identifies an OS-level execution boundary. Profiles may share
// one binary, but each is deployed as a separate process and service identity.
type RunnerProfile string

const (
	ProfileDiagnostics     RunnerProfile = "diagnostics"
	ProfileNetworkObserver RunnerProfile = "network-observer"
	ProfileLogReader       RunnerProfile = "log-reader"
	ProfileServiceManager  RunnerProfile = "service-manager"
)

// ExecutorKind selects a typed backend. It is configuration, never caller input.
type ExecutorKind string

const (
	ExecutorProcess  ExecutorKind = "process"
	ExecutorSystemd  ExecutorKind = "systemd"
	ExecutorTCPProbe ExecutorKind = "tcp-probe"
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
	// Default is used when an optional parameter is omitted.
	Default string   `json:"default,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Min     *int     `json:"min,omitempty"`
	Max     *int     `json:"max,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	// ValueMap exposes stable operator-facing aliases while translating them
	// to fixed, catalog-owned argv values such as approved log paths.
	ValueMap map[string]string `json:"value_map,omitempty"`
}

// OperationDefinition describes one safe operation SafeTerm exposes to
// users, and how internal/translation maps it to a real system command
// (M6). ArgsTemplate remains inert data in this package; translation.New
// validates and compiles its whole-token placeholders (ADR-007).
type ArgumentGroup struct {
	When         string   `json:"when"`
	ArgsTemplate []string `json:"args_template"`
}

type OperationDefinition struct {
	RequiredAny            []string                    `json:"required_any,omitempty"`
	ArgumentGroups         []ArgumentGroup             `json:"argument_groups,omitempty"`
	ArgumentGroupSeparator []string                    `json:"argument_group_separator,omitempty"`
	Name                   string                      `json:"name"`
	Description            string                      `json:"description"`
	RunnerProfile          RunnerProfile               `json:"runner_profile"`
	ExecutorKind           ExecutorKind                `json:"executor_kind"`
	ExecutableRef          string                      `json:"executable_ref,omitempty"`
	ArgsTemplate           []string                    `json:"args_template,omitempty"`
	Systemd                *SystemdOperationDefinition `json:"systemd,omitempty"`
	TCPProbe               *TCPProbeDefinition         `json:"tcp_probe,omitempty"`
	Parameters             []ParameterDefinition       `json:"parameters"`
	LongRunning            bool                        `json:"long_running"`
}

type SystemdOperationDefinition struct {
	Unit           string   `json:"unit,omitempty"`
	Units          []string `json:"units,omitempty"`
	Alias          string   `json:"alias,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	Action         string   `json:"action"`
}

type ExecutableDefinition struct {
	ID              string          `json:"id"`
	Path            string          `json:"path"`
	SHA256          string          `json:"sha256"`
	AllowedProfiles []RunnerProfile `json:"allowed_profiles"`
}

type ProfileDefinition struct {
	Template                RunnerProfile `json:"template,omitempty"`
	TimeoutSeconds          int           `json:"timeout_seconds,omitempty"`
	OutputLimitBytes        int           `json:"output_limit_bytes,omitempty"`
	MaxSessions             int           `json:"max_sessions,omitempty"`
	IdleTimeoutSeconds      int           `json:"idle_timeout_seconds,omitempty"`
	Name                    RunnerProfile `json:"name"`
	ExecutorKind            ExecutorKind  `json:"executor_kind"`
	ControllerExecutableRef string        `json:"controller_executable_ref,omitempty"`
	ControllerArgsPrefix    []string      `json:"controller_args_prefix,omitempty"`
	SocketPath              string        `json:"socket_path"`
	RunAsUser               string        `json:"run_as_user"`
	Capabilities            []string      `json:"capabilities"`
	MemoryLimitMiB          int           `json:"memory_limit_mib"`
	ProcessLimit            int           `json:"process_limit"`
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
	Version     string
	Operations  []OperationDefinition
	Executables []ExecutableDefinition
	Profiles    []ProfileDefinition
	Roles       []RoleDefinition
	Policies    []PolicyDefinition
}

// TCPProbeDefinition configures a single literal-IP TCP connect attempt, without application data.
type TCPProbeDefinition struct {
	TimeoutSeconds int `json:"timeout_seconds"`
}
