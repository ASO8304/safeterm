package configmanager

import "time"

const ProtocolVersion = 1

type Format string

const (
	FormatINI  Format = "ini"
	FormatJSON Format = "json"
)

type AccessMode string

const (
	AccessReadOnly  AccessMode = "read-only"
	AccessReadWrite AccessMode = "read-write"
)

type FieldType string

const (
	FieldString  FieldType = "string"
	FieldInteger FieldType = "integer"
	FieldBoolean FieldType = "boolean"
	FieldEnum    FieldType = "enum"
	FieldPath    FieldType = "path"
)

type Action string

const (
	ActionRead     Action = "read"
	ActionPropose  Action = "propose"
	ActionApply    Action = "apply"
	ActionActivate Action = "activate"
	ActionRollback Action = "rollback"
)

type Actor struct {
	ConsoleSession string   `json:"console_session,omitempty"`
	BrokerType     string   `json:"broker_type"`
	UserID         string   `json:"user_id"`
	Roles          []string `json:"roles"`
}

type Permissions struct {
	ReadRoles     []string `json:"read_roles"`
	ProposeRoles  []string `json:"propose_roles"`
	ApplyRoles    []string `json:"apply_roles"`
	ActivateRoles []string `json:"activate_roles"`
	RollbackRoles []string `json:"rollback_roles"`
}

type PathPolicy struct {
	AllowedPrefixes []string `json:"allowed_prefixes"`
	MustExist       bool     `json:"must_exist"`
	FileType        string   `json:"file_type,omitempty"`
	AllowSymlink    bool     `json:"allow_symlink"`
	ExpectedUID     *int     `json:"expected_uid,omitempty"`
	ExpectedMode    string   `json:"expected_mode,omitempty"`
}

type FieldSchema struct {
	Section   string      `json:"section"`
	Key       string      `json:"key"`
	Type      FieldType   `json:"type"`
	Required  bool        `json:"required"`
	Secret    bool        `json:"secret"`
	ReadOnly  bool        `json:"read_only"`
	Enum      []string    `json:"enum,omitempty"`
	Min       *int64      `json:"min,omitempty"`
	Max       *int64      `json:"max,omitempty"`
	MinLength *int        `json:"min_length,omitempty"`
	MaxLength *int        `json:"max_length,omitempty"`
	Pattern   string      `json:"pattern,omitempty"`
	Path      *PathPolicy `json:"path,omitempty"`
}

type CrossFieldRule struct {
	Type         string `json:"type"`
	Section      string `json:"section"`
	Key          string `json:"key"`
	OtherSection string `json:"other_section"`
	OtherKey     string `json:"other_key"`
}

type BackupPolicy struct {
	Enabled bool `json:"enabled"`
	Keep    int  `json:"keep"`
}

type Resource struct {
	ID                      string           `json:"id"`
	Description             string           `json:"description"`
	Profile                 string           `json:"profile"`
	Path                    string           `json:"path"`
	Format                  Format           `json:"format"`
	Access                  AccessMode       `json:"access"`
	Fields                  []FieldSchema    `json:"fields"`
	CrossFieldRules         []CrossFieldRule `json:"cross_field_rules,omitempty"`
	ValidatorID             string           `json:"validator_id,omitempty"`
	ActivationOperation     string           `json:"activation_operation,omitempty"`
	HealthCheckOperation    string           `json:"health_check_operation,omitempty"`
	RollbackOnFailure       bool             `json:"rollback_on_failure"`
	RequireSeparateApproval bool             `json:"require_separate_approval"`
	Backup                  BackupPolicy     `json:"backup"`
	Permissions             Permissions      `json:"permissions"`
}

type Registry struct {
	Version   string     `json:"version"`
	Resources []Resource `json:"resources"`
}

type ResourceSummary struct {
	ID                      string     `json:"id"`
	Description             string     `json:"description"`
	Profile                 string     `json:"profile"`
	Format                  Format     `json:"format"`
	Access                  AccessMode `json:"access"`
	RequireSeparateApproval bool       `json:"require_separate_approval"`
}

type FieldDescription struct {
	Section   string    `json:"section"`
	Key       string    `json:"key"`
	Type      FieldType `json:"type"`
	Required  bool      `json:"required"`
	Secret    bool      `json:"secret"`
	ReadOnly  bool      `json:"read_only"`
	Enum      []string  `json:"enum,omitempty"`
	Min       *int64    `json:"min,omitempty"`
	Max       *int64    `json:"max,omitempty"`
	MinLength *int      `json:"min_length,omitempty"`
	MaxLength *int      `json:"max_length,omitempty"`
	Pattern   string    `json:"pattern,omitempty"`
}

type ResourceDescription struct {
	Resource ResourceSummary    `json:"resource"`
	Fields   []FieldDescription `json:"fields"`
}

type FieldValue struct {
	Section  string `json:"section"`
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	Redacted bool   `json:"redacted"`
}

type ReadResult struct {
	ResourceID string       `json:"resource_id"`
	Version    string       `json:"version"`
	Fields     []FieldValue `json:"fields"`
}

type Patch struct {
	ResourceID  string `json:"resource_id"`
	Section     string `json:"section"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	BaseVersion string `json:"base_version"`
}

type ValidationResult struct {
	Valid           bool   `json:"valid"`
	NormalizedValue string `json:"normalized_value,omitempty"`
	Message         string `json:"message,omitempty"`
}

type PreviewResult struct {
	Token      string    `json:"token"`
	ResourceID string    `json:"resource_id"`
	Diff       string    `json:"diff"`
	ExpiresAt  time.Time `json:"expires_at"`
	Secret     bool      `json:"secret"`
}

type ApplyResult struct {
	ResourceID        string `json:"resource_id"`
	PreviousVersion   string `json:"previous_version"`
	Version           string `json:"version"`
	ActivationOutcome string `json:"activation_outcome,omitempty"`
	RollbackOutcome   string `json:"rollback_outcome,omitempty"`
}

type RollbackResult struct {
	ResourceID      string `json:"resource_id"`
	PreviousVersion string `json:"previous_version"`
	Version         string `json:"version"`
}
