// Package isolation enforces the M8 capability-profile boundary around Agent Core.
package isolation

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"safeterm/internal/config"
)

type Snapshot struct {
	config     *config.Config
	routes     map[string]config.RunnerProfile
	profiles   map[config.RunnerProfile]config.ProfileDefinition
	operations map[config.RunnerProfile]map[string]bool
}

func NewSnapshot(candidate *config.Config) (*Snapshot, error) {
	if err := config.ValidateConfig(candidate); err != nil {
		return nil, fmt.Errorf("isolation: invalid catalog snapshot: %w", err)
	}
	cloned, err := cloneConfig(candidate)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{
		config: cloned, routes: map[string]config.RunnerProfile{},
		profiles:   map[config.RunnerProfile]config.ProfileDefinition{},
		operations: map[config.RunnerProfile]map[string]bool{},
	}
	for _, profile := range cloned.Profiles {
		snapshot.profiles[profile.Name] = profile
		snapshot.operations[profile.Name] = map[string]bool{}
	}
	for _, operation := range cloned.Operations {
		snapshot.routes[operation.Name] = operation.RunnerProfile
		snapshot.operations[operation.RunnerProfile][operation.Name] = true
	}
	return snapshot, nil
}

func cloneConfig(source *config.Config) (*config.Config, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("isolation: clone catalog: %w", err)
	}
	var cloned config.Config
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("isolation: clone catalog: %w", err)
	}
	return &cloned, nil
}

func (s *Snapshot) Version() string {
	if s == nil || s.config == nil {
		return ""
	}
	return s.config.Version
}

func (s *Snapshot) Route(operation string) (config.RunnerProfile, bool) {
	if s == nil {
		return "", false
	}
	profile, ok := s.routes[operation]
	return profile, ok
}

func (s *Snapshot) Allows(profile config.RunnerProfile, operation string) bool {
	return s != nil && s.operations[profile] != nil && s.operations[profile][operation]
}

func (s *Snapshot) Profile(profile config.RunnerProfile) (config.ProfileDefinition, bool) {
	if s == nil {
		return config.ProfileDefinition{}, false
	}
	value, ok := s.profiles[profile]
	value.Capabilities = append([]string(nil), value.Capabilities...)
	return value, ok
}

// ProfileConfig returns a defensive, least-privilege configuration containing
// only one profile and its operations, executable bindings, and policy grants.
func (s *Snapshot) ProfileConfig(profile config.RunnerProfile) (*config.Config, error) {
	definition, ok := s.Profile(profile)
	if !ok {
		return nil, fmt.Errorf("isolation: unknown profile %q", profile)
	}
	result := &config.Config{Version: s.config.Version, Profiles: []config.ProfileDefinition{definition}, Roles: append([]config.RoleDefinition(nil), s.config.Roles...)}
	allowedOps := map[string]bool{}
	executableIDs := map[string]bool{}
	if definition.ControllerExecutableRef != "" {
		executableIDs[definition.ControllerExecutableRef] = true
	}
	for _, operation := range s.config.Operations {
		if operation.RunnerProfile == profile {
			result.Operations = append(result.Operations, operation)
			allowedOps[operation.Name] = true
			if operation.ExecutableRef != "" {
				executableIDs[operation.ExecutableRef] = true
			}
		}
	}
	for _, executable := range s.config.Executables {
		if executableIDs[executable.ID] {
			executable.AllowedProfiles = []config.RunnerProfile{profile}
			result.Executables = append(result.Executables, executable)
		}
	}
	for _, policy := range s.config.Policies {
		filtered := config.PolicyDefinition{Role: policy.Role}
		for _, operation := range policy.Operations {
			if allowedOps[operation] {
				filtered.Operations = append(filtered.Operations, operation)
			}
		}
		result.Policies = append(result.Policies, filtered)
	}
	if err := config.ValidateConfig(result); err != nil {
		return nil, fmt.Errorf("isolation: invalid profile snapshot: %w", err)
	}
	return cloneConfig(result)
}

type CatalogVerifier func(*config.Config) error

type Store struct {
	current  atomic.Pointer[Snapshot]
	mu       sync.Mutex
	previous *Snapshot
	verifier CatalogVerifier
}

func NewStore(initial *config.Config) (*Store, error) {
	return newStore(initial, nil)
}

// NewVerifiedStore requires every initial and replacement snapshot to pass an
// external integrity check, such as a detached catalog signature.
func NewVerifiedStore(initial *config.Config, verifier CatalogVerifier) (*Store, error) {
	if verifier == nil {
		return nil, fmt.Errorf("isolation: catalog verifier is required")
	}
	return newStore(initial, verifier)
}

func newStore(initial *config.Config, verifier CatalogVerifier) (*Store, error) {
	if verifier != nil {
		if err := verifier(initial); err != nil {
			return nil, fmt.Errorf("isolation: verify initial catalog: %w", err)
		}
	}
	snapshot, err := NewSnapshot(initial)
	if err != nil {
		return nil, err
	}
	store := &Store{verifier: verifier}
	store.current.Store(snapshot)
	return store, nil
}

func (s *Store) Current() *Snapshot {
	if s == nil {
		return nil
	}
	return s.current.Load()
}

// Activate validates and clones before one atomic pointer swap. Failure leaves
// the previously active snapshot untouched.
func (s *Store) Activate(candidate *config.Config) error {
	if s == nil {
		return fmt.Errorf("isolation: nil catalog store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.verifier != nil {
		if err := s.verifier(candidate); err != nil {
			return fmt.Errorf("isolation: verify replacement catalog: %w", err)
		}
	}
	snapshot, err := NewSnapshot(candidate)
	if err != nil {
		return err
	}
	s.previous = s.current.Load()
	s.current.Store(snapshot)
	return nil
}

// Rollback atomically restores the last previously active, already-verified
// snapshot. It never parses or trusts new external input.
func (s *Store) Rollback() error {
	if s == nil {
		return fmt.Errorf("isolation: nil catalog store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.previous == nil {
		return fmt.Errorf("isolation: no previous catalog snapshot")
	}
	s.current.Store(s.previous)
	s.previous = nil
	return nil
}
