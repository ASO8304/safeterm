// Package config will later be responsible for loading and validating
// operations.yaml, roles.yaml and policies.yaml (see docs/decisions/ADR-001).
// For M0 it only exposes a Status() function so the project has a real,
// testable unit from day one.
package config

// Status reports the current bootstrap status of the configuration package.
// This will be replaced by real config-loading logic in Phase M3.
func Status() string {
	return "not-yet-configured (M0 placeholder)"
}
