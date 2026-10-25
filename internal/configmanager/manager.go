package configmanager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"safeterm/internal/audit"
)

const (
	defaultProposalTTL = 5 * time.Minute
	maxProposals       = 1024
)

type ApplicationValidator interface {
	Validate(context.Context, Resource, []byte) error
}

type TypedActivator interface {
	Run(context.Context, Actor, string) error
}

type ManagerOptions struct {
	FileWriter  FileWriter
	ProposalTTL time.Duration
	Now         func() time.Time
}

type proposal struct {
	id              string
	token           string
	actor           Actor
	resourceID      string
	patch           Patch
	normalizedValue string
	rendered        []byte
	diff            string
	secret          bool
	expiresAt       time.Time
}

type managedResource struct {
	definition Resource
	mu         sync.Mutex
}

type Manager struct {
	writer     FileWriter
	profile    string
	version    string
	resources  map[string]*managedResource
	validators map[string]ApplicationValidator
	activator  TypedActivator
	audit      audit.Sink
	ttl        time.Duration
	now        func() time.Time
	proposalMu sync.Mutex
	proposals  map[string]proposal
}

func NewManager(profile string, registry *Registry, validators map[string]ApplicationValidator, activator TypedActivator, sink audit.Sink, options ManagerOptions) (*Manager, error) {
	if !identifierPattern.MatchString(profile) || sink == nil {
		return nil, fmt.Errorf("configmanager: valid profile and audit sink are required")
	}
	if err := ValidateRegistry(registry); err != nil {
		return nil, err
	}
	selected := resourcesForProfile(registry, profile)

	if options.ProposalTTL == 0 {
		options.ProposalTTL = defaultProposalTTL
	}
	if options.ProposalTTL < time.Second || options.ProposalTTL > time.Hour {
		return nil, fmt.Errorf("configmanager: proposal TTL must be between one second and one hour")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	manager := &Manager{
		writer: options.FileWriter, profile: profile, version: registry.Version, resources: map[string]*managedResource{},
		validators: map[string]ApplicationValidator{}, activator: activator, audit: sink,
		ttl: options.ProposalTTL, now: options.Now, proposals: map[string]proposal{},
	}
	for id, validator := range validators {
		if id == "" || validator == nil {
			return nil, fmt.Errorf("configmanager: invalid application validator %q", id)
		}
		manager.validators[id] = validator
	}
	for _, resource := range selected {
		if resource.ValidatorID != "" && manager.validators[resource.ValidatorID] == nil {
			return nil, fmt.Errorf("configmanager: resource %q references unavailable validator %q", resource.ID, resource.ValidatorID)
		}
		if (resource.ActivationOperation != "" || resource.HealthCheckOperation != "") && activator == nil {
			return nil, fmt.Errorf("configmanager: resource %q requires an activator", resource.ID)
		}
		data, _, err := manager.readFile(resource)
		if err != nil {
			return nil, fmt.Errorf("configmanager: verify resource %q: %w", resource.ID, err)
		}
		document, err := parseDocument(resource.Format, data)
		if err != nil {
			return nil, fmt.Errorf("configmanager: parse resource %q: %w", resource.ID, err)
		}
		if err := validateCompleteDocument(resource, document); err != nil {
			return nil, fmt.Errorf("configmanager: validate resource %q: %w", resource.ID, err)
		}
		if resource.ValidatorID != "" {
			if err := manager.validators[resource.ValidatorID].Validate(context.Background(), resource, data); err != nil {
				return nil, fmt.Errorf("configmanager: application validation for resource %q: %w", resource.ID, err)
			}
		}
		manager.resources[resource.ID] = &managedResource{definition: resource}
	}
	return manager, nil
}

func (m *Manager) List(ctx context.Context, actor Actor) ([]ResourceSummary, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	result := []ResourceSummary{}
	for _, managed := range m.resources {
		resource := managed.definition
		if authorized(actor.Roles, resource.Permissions.ReadRoles) {
			result = append(result, summary(resource))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (m *Manager) Describe(ctx context.Context, actor Actor, resourceID string) (ResourceDescription, error) {
	resource, err := m.authorize(actor, resourceID, ActionRead)
	if err != nil {
		return ResourceDescription{}, err
	}
	fields := make([]FieldDescription, 0, len(resource.Fields))
	for _, field := range resource.Fields {
		fields = append(fields, FieldDescription{
			Description: field.Description, Section: field.Section, Key: field.Key, Type: field.Type, Required: field.Required, AllowEmpty: field.AllowEmpty,
			Secret: field.Secret, ReadOnly: field.ReadOnly, Enum: append([]string(nil), field.Enum...), Min: cloneInt64(field.Min),
			Max: cloneInt64(field.Max), MinLength: cloneInt(field.MinLength), MaxLength: cloneInt(field.MaxLength), Pattern: field.Pattern,
		})
	}
	return ResourceDescription{Resource: summary(resource), Fields: fields}, nil
}

func (m *Manager) Read(ctx context.Context, actor Actor, resourceID string) (ReadResult, error) {
	resource, err := m.authorize(actor, resourceID, ActionRead)
	if err != nil {
		return ReadResult{}, err
	}
	managed := m.resources[resourceID]
	managed.mu.Lock()
	defer managed.mu.Unlock()
	data, _, err := m.readFile(resource)
	if err != nil {
		return ReadResult{}, err
	}
	document, err := parseDocument(resource.Format, data)
	if err != nil {
		return ReadResult{}, err
	}
	if err := validateCompleteDocument(resource, document); err != nil {
		return ReadResult{}, err
	}
	result := ReadResult{ResourceID: resource.ID, Version: contentVersion(data)}
	for _, field := range resource.Fields {
		value, exists := document.Get(field.Section, field.Key)
		if !exists {
			continue
		}
		entry := FieldValue{Section: field.Section, Key: field.Key, Redacted: field.Secret}
		if field.Secret {
			entry.Value = "[REDACTED]"
		} else {
			entry.Value = scalarString(value)
		}
		result.Fields = append(result.Fields, entry)
	}
	if err := m.record(ctx, actor, resource, ActionRead, "read", "", result.Version, result.Version, "", ""); err != nil {
		return ReadResult{}, err
	}
	return result, nil
}

func (m *Manager) ValidatePatch(ctx context.Context, actor Actor, patch Patch) (ValidationResult, error) {
	if _, err := m.authorize(actor, patch.ResourceID, ActionPropose); err != nil {
		return ValidationResult{}, err
	}
	prepared, _, err := m.prepare(ctx, actor, patch)
	if err != nil {
		if resource := m.resources[patch.ResourceID]; resource != nil {
			if auditErr := m.record(ctx, actor, resource.definition, ActionPropose, "validation-rejected", patchCoordinate(patch), patch.BaseVersion, "", "", ""); auditErr != nil {
				return ValidationResult{}, auditErr
			}
		}
		return ValidationResult{Valid: false, Message: err.Error()}, nil
	}
	return ValidationResult{Valid: true, NormalizedValue: redactIf(prepared.secret, prepared.normalizedValue)}, nil
}

func (m *Manager) Preview(ctx context.Context, actor Actor, patch Patch) (PreviewResult, error) {
	prepared, resource, err := m.prepare(ctx, actor, patch)
	if err != nil {
		return PreviewResult{}, err
	}
	token, id, err := newProposalToken()
	if err != nil {
		return PreviewResult{}, err
	}
	prepared.id = id
	prepared.token = token
	prepared.actor = cloneActor(actor)
	prepared.expiresAt = m.now().UTC().Add(m.ttl)
	m.proposalMu.Lock()
	m.pruneProposalsLocked()
	if len(m.proposals) >= maxProposals {
		m.proposalMu.Unlock()
		return PreviewResult{}, fmt.Errorf("configmanager: proposal capacity reached")
	}
	m.proposals[token] = prepared
	m.proposalMu.Unlock()
	if err := m.record(ctx, actor, resource, ActionPropose, "preview-created", patchCoordinate(patch), patch.BaseVersion, "", id, ""); err != nil {
		m.proposalMu.Lock()
		delete(m.proposals, token)
		m.proposalMu.Unlock()
		return PreviewResult{}, err
	}
	return PreviewResult{Token: token, ResourceID: resource.ID, Diff: prepared.diff, ExpiresAt: prepared.expiresAt, Secret: prepared.secret}, nil
}

func (m *Manager) Apply(ctx context.Context, actor Actor, token string) (ApplyResult, error) {
	if err := validateActor(actor); err != nil {
		return ApplyResult{}, err
	}
	m.proposalMu.Lock()
	prepared, exists := m.proposals[token]
	m.proposalMu.Unlock()
	if !exists || token == "" {
		return ApplyResult{}, fmt.Errorf("configmanager: proposal not found")
	}
	resource, err := m.authorize(actor, prepared.resourceID, ActionApply)
	if err != nil {
		return ApplyResult{}, err
	}
	if !m.now().UTC().Before(prepared.expiresAt) {
		m.proposalMu.Lock()
		delete(m.proposals, token)
		m.proposalMu.Unlock()
		return ApplyResult{}, fmt.Errorf("configmanager: proposal expired")
	}
	if actor.UserID != prepared.actor.UserID || actor.BrokerType != prepared.actor.BrokerType || actor.ConsoleSession != prepared.actor.ConsoleSession {
		return ApplyResult{}, fmt.Errorf("configmanager: proposal is bound to its creator")
	}
	m.proposalMu.Lock()
	if _, stillExists := m.proposals[token]; !stillExists {
		m.proposalMu.Unlock()
		return ApplyResult{}, fmt.Errorf("configmanager: proposal already consumed")
	}
	delete(m.proposals, token)
	m.proposalMu.Unlock()
	if resource.ActivationOperation != "" && !authorized(actor.Roles, resource.Permissions.ActivateRoles) {
		return ApplyResult{}, fmt.Errorf("configmanager: actor is not authorized to activate resource %q", resource.ID)
	}
	managed := m.resources[resource.ID]
	managed.mu.Lock()
	defer managed.mu.Unlock()
	current, info, err := m.readFile(resource)
	if err != nil {
		return ApplyResult{}, err
	}
	currentVersion := contentVersion(current)
	if currentVersion != prepared.patch.BaseVersion {
		_ = m.record(ctx, actor, resource, ActionApply, "apply-rejected", patchCoordinate(prepared.patch), prepared.patch.BaseVersion, currentVersion, prepared.id, actor.UserID)
		return ApplyResult{}, fmt.Errorf("configmanager: version conflict")
	}
	if contentVersion(prepared.rendered) == currentVersion {
		return ApplyResult{}, fmt.Errorf("configmanager: proposal makes no change")
	}
	if err := m.record(ctx, actor, resource, ActionApply, "apply-authorized", patchCoordinate(prepared.patch), currentVersion, contentVersion(prepared.rendered), prepared.id, actor.UserID); err != nil {
		return ApplyResult{}, err
	}
	backup, err := m.commitFile(resource, prepared.patch, prepared.rendered, current, info)
	if err != nil {
		_ = m.record(ctx, actor, resource, ActionApply, "apply-rejected", patchCoordinate(prepared.patch), currentVersion, "", prepared.id, actor.UserID)
		return ApplyResult{}, err
	}
	newVersion := contentVersion(prepared.rendered)
	result := ApplyResult{ResourceID: resource.ID, PreviousVersion: currentVersion, Version: newVersion}
	if resource.ActivationOperation != "" {
		if err := m.activator.Run(ctx, actor, resource.ActivationOperation); err != nil {
			result.ActivationOutcome = "failed"
			_ = m.record(ctx, actor, resource, ActionActivate, "activation-failed", patchCoordinate(prepared.patch), currentVersion, newVersion, prepared.id, actor.UserID)
			if resource.RollbackOnFailure && backup != "" {
				if rollbackErr := m.restoreFile(resource, backup, newVersion); rollbackErr != nil {
					result.RollbackOutcome = "failed"
					_ = m.record(ctx, actor, resource, ActionRollback, "rollback-failed", patchCoordinate(prepared.patch), newVersion, currentVersion, prepared.id, actor.UserID)
					return result, fmt.Errorf("configmanager: activation failed: %v; rollback failed: %w", err, rollbackErr)
				}
				result.RollbackOutcome = "succeeded"
				result.Version = currentVersion
				_ = m.record(ctx, actor, resource, ActionRollback, "rollback-succeeded", patchCoordinate(prepared.patch), newVersion, currentVersion, prepared.id, actor.UserID)
			}
			return result, fmt.Errorf("configmanager: activation failed: %w", err)
		}
		result.ActivationOutcome = "succeeded"
		if resource.HealthCheckOperation != "" {
			if err := m.activator.Run(ctx, actor, resource.HealthCheckOperation); err != nil {
				result.ActivationOutcome = "health-check-failed"
				_ = m.record(ctx, actor, resource, ActionActivate, "activation-failed", patchCoordinate(prepared.patch), currentVersion, newVersion, prepared.id, actor.UserID)
				if resource.RollbackOnFailure && backup != "" {
					if rollbackErr := m.restoreFile(resource, backup, newVersion); rollbackErr == nil {
						result.RollbackOutcome, result.Version = "succeeded", currentVersion
						_ = m.record(ctx, actor, resource, ActionRollback, "rollback-succeeded", patchCoordinate(prepared.patch), newVersion, currentVersion, prepared.id, actor.UserID)
					} else {
						result.RollbackOutcome = "failed"
						_ = m.record(ctx, actor, resource, ActionRollback, "rollback-failed", patchCoordinate(prepared.patch), newVersion, currentVersion, prepared.id, actor.UserID)
						return result, fmt.Errorf("configmanager: health check failed: %v; rollback failed: %w", err, rollbackErr)
					}
				}
				return result, fmt.Errorf("configmanager: health check failed: %w", err)
			}
		}
		_ = m.record(ctx, actor, resource, ActionActivate, "activation-succeeded", patchCoordinate(prepared.patch), currentVersion, newVersion, prepared.id, actor.UserID)
	}
	if err := m.record(ctx, actor, resource, ActionApply, "applied", patchCoordinate(prepared.patch), currentVersion, result.Version, prepared.id, actor.UserID); err != nil {
		if backup != "" {
			_ = m.restoreFile(resource, backup, newVersion)
		}
		return ApplyResult{}, err
	}
	return result, nil
}

func (m *Manager) Rollback(ctx context.Context, actor Actor, resourceID string) (RollbackResult, error) {
	resource, err := m.authorize(actor, resourceID, ActionRollback)
	if err != nil {
		return RollbackResult{}, err
	}
	managed := m.resources[resourceID]
	managed.mu.Lock()
	defer managed.mu.Unlock()
	current, _, err := m.readFile(resource)
	if err != nil {
		return RollbackResult{}, err
	}
	backup, err := m.latestFileBackup(resource)
	if err != nil {
		return RollbackResult{}, err
	}
	before := contentVersion(current)
	if err := m.record(ctx, actor, resource, ActionRollback, "rollback-authorized", "", before, "", "", actor.UserID); err != nil {
		return RollbackResult{}, err
	}
	if err := m.restoreFile(resource, backup, before); err != nil {
		_ = m.record(ctx, actor, resource, ActionRollback, "rollback-failed", "", before, "", "", actor.UserID)
		return RollbackResult{}, err
	}
	afterData, _, err := m.readFile(resource)
	if err != nil {
		return RollbackResult{}, err
	}
	after := contentVersion(afterData)
	if err := m.record(ctx, actor, resource, ActionRollback, "rollback-succeeded", "", before, after, "", actor.UserID); err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{ResourceID: resource.ID, PreviousVersion: before, Version: after}, nil
}

func (m *Manager) prepare(ctx context.Context, actor Actor, patch Patch) (proposal, Resource, error) {
	resource, err := m.authorize(actor, patch.ResourceID, ActionPropose)
	if err != nil {
		return proposal{}, Resource{}, err
	}
	if resource.Access != AccessReadWrite || patch.BaseVersion == "" {
		return proposal{}, Resource{}, fmt.Errorf("configmanager: writable resource and base version are required")
	}
	managed := m.resources[resource.ID]
	managed.mu.Lock()
	defer managed.mu.Unlock()
	data, _, err := m.readFile(resource)
	if err != nil {
		return proposal{}, Resource{}, err
	}
	if contentVersion(data) != patch.BaseVersion {
		return proposal{}, Resource{}, fmt.Errorf("configmanager: version conflict")
	}
	rendered, diff, normalized, secret, err := renderPatch(resource, data, patch)
	if err != nil {
		return proposal{}, Resource{}, err
	}
	if resource.ValidatorID != "" {
		if err := m.validators[resource.ValidatorID].Validate(ctx, resource, rendered); err != nil {
			return proposal{}, Resource{}, fmt.Errorf("configmanager: application validation failed: %w", err)
		}
	}
	patch.Changes = append([]FieldChange(nil), patch.Changes...)
	return proposal{resourceID: resource.ID, patch: patch, normalizedValue: normalized, rendered: rendered, diff: diff, secret: secret}, resource, nil
}

func (m *Manager) authorize(actor Actor, resourceID string, action Action) (Resource, error) {
	if err := validateActor(actor); err != nil {
		return Resource{}, err
	}
	managed := m.resources[resourceID]
	if managed == nil {
		return Resource{}, fmt.Errorf("configmanager: resource not found")
	}
	resource := managed.definition
	var allowed []string
	switch action {
	case ActionRead:
		allowed = resource.Permissions.ReadRoles
	case ActionPropose:
		allowed = resource.Permissions.ProposeRoles
	case ActionApply:
		allowed = resource.Permissions.ApplyRoles
	case ActionActivate:
		allowed = resource.Permissions.ActivateRoles
	case ActionRollback:
		allowed = resource.Permissions.RollbackRoles
	}
	if !authorized(actor.Roles, allowed) {
		return Resource{}, fmt.Errorf("configmanager: actor is not authorized for %s", action)
	}
	return resource, nil
}

func validateActor(actor Actor) error {
	if actor.BrokerType == "" || actor.UserID == "" || len(actor.Roles) == 0 {
		return fmt.Errorf("configmanager: authenticated actor is required")
	}
	return nil
}

func authorized(actorRoles, allowedRoles []string) bool {
	for _, actorRole := range actorRoles {
		for _, allowedRole := range allowedRoles {
			if actorRole == allowedRole {
				return true
			}
		}
	}
	return false
}

func (m *Manager) record(ctx context.Context, actor Actor, resource Resource, action Action, outcome, field, baseVersion, newVersion, proposalID, approvalID string) error {
	event := audit.Event{
		SchemaVersion: audit.SchemaVersion, RequestID: randomID(), CatalogVersion: m.version,
		Component: audit.ComponentConfigManager, Profile: m.profile, Operation: string(action), Outcome: outcome,
		BrokerType: actor.BrokerType, UserID: actor.UserID, ClaimedRoles: append([]string(nil), actor.Roles...),
		ResourceID: resource.ID, Field: field, BaseVersion: baseVersion, NewVersion: newVersion,
		ProposalID: proposalID, ApprovalUserID: approvalID, OccurredAt: m.now().UTC(),
	}
	return m.audit.Record(ctx, event)
}

func summary(resource Resource) ResourceSummary {
	return ResourceSummary{Contexts: cloneContexts(resource.Contexts), ID: resource.ID, Description: resource.Description, Profile: resource.Profile, Format: resource.Format, Access: resource.Access}
}

func findField(resource Resource, section, key string) (FieldSchema, bool) {
	for _, field := range resource.Fields {
		if field.Section == section && field.Key == key {
			return field, true
		}
	}
	return FieldSchema{}, false
}

func contentVersion(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func newProposalToken() (string, string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", "", fmt.Errorf("configmanager: generate proposal token: %w", err)
	}
	token := hex.EncodeToString(data)
	idDigest := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(idDigest[:8]), nil
}

func randomID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data)
}

func renderDiff(field FieldSchema, oldValue, newValue string) string {
	coordinate := field.Key
	if field.Section != "" {
		coordinate = field.Section + "." + field.Key
	}
	if field.Secret {
		oldValue, newValue = "[REDACTED]", "[REDACTED]"
	}
	return fmt.Sprintf("- %s = %s\n+ %s = %s", coordinate, oldValue, coordinate, newValue)
}

func redactIf(secret bool, value string) string {
	if secret {
		return "[REDACTED]"
	}
	return value
}

func cloneActor(actor Actor) Actor {
	actor.Roles = append([]string(nil), actor.Roles...)
	return actor
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func (m *Manager) pruneProposalsLocked() {
	now := m.now().UTC()
	for token, proposal := range m.proposals {
		if !now.Before(proposal.expiresAt) {
			delete(m.proposals, token)
		}
	}
}

func fieldCoordinate(section, key string) string {
	return strings.TrimPrefix(section+"."+key, ".")
}
