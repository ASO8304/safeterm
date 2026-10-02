package broker

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"safeterm/internal/configmanager"
)

// ConfigBroker routes opaque resource IDs to isolated Config Manager clients.
// The caller never supplies a socket, filesystem path, parser, or executable.
type ConfigBroker struct {
	clients []configmanager.Service
	mu      sync.Mutex
	tokens  map[string]configmanager.Service
}

func NewConfigBroker(clients []configmanager.Service) (*ConfigBroker, error) {
	if len(clients) == 0 {
		return nil, fmt.Errorf("broker: at least one Config Manager is required")
	}
	for _, client := range clients {
		if client == nil {
			return nil, fmt.Errorf("broker: nil Config Manager client")
		}
	}
	return &ConfigBroker{clients: append([]configmanager.Service(nil), clients...), tokens: map[string]configmanager.Service{}}, nil
}

func configActor(principal Principal) configmanager.Actor {
	return configmanager.Actor{BrokerType: "ssh", UserID: principal.UserID, Roles: append([]string(nil), principal.Roles...)}
}

func (b *ConfigBroker) List(ctx context.Context, principal Principal) ([]configmanager.ResourceSummary, error) {
	all := []configmanager.ResourceSummary{}
	seen := map[string]bool{}
	for _, client := range b.clients {
		items, err := client.List(ctx, configActor(principal))
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if seen[item.ID] {
				return nil, fmt.Errorf("broker: duplicate config resource %q", item.ID)
			}
			seen[item.ID] = true
			all = append(all, item)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}

func (b *ConfigBroker) clientFor(ctx context.Context, principal Principal, id string) (configmanager.Service, error) {
	for _, client := range b.clients {
		items, err := client.List(ctx, configActor(principal))
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ID == id {
				return client, nil
			}
		}
	}
	return nil, fmt.Errorf("config resource %q is unknown or unauthorized", id)
}

func (b *ConfigBroker) Describe(ctx context.Context, p Principal, id string) (configmanager.ResourceDescription, error) {
	c, e := b.clientFor(ctx, p, id)
	if e != nil {
		return configmanager.ResourceDescription{}, e
	}
	return c.Describe(ctx, configActor(p), id)
}
func (b *ConfigBroker) Read(ctx context.Context, p Principal, id string) (configmanager.ReadResult, error) {
	c, e := b.clientFor(ctx, p, id)
	if e != nil {
		return configmanager.ReadResult{}, e
	}
	return c.Read(ctx, configActor(p), id)
}
func (b *ConfigBroker) ValidatePatch(ctx context.Context, p Principal, patch configmanager.Patch) (configmanager.ValidationResult, error) {
	c, e := b.clientFor(ctx, p, patch.ResourceID)
	if e != nil {
		return configmanager.ValidationResult{}, e
	}
	return c.ValidatePatch(ctx, configActor(p), patch)
}
func (b *ConfigBroker) Preview(ctx context.Context, p Principal, patch configmanager.Patch) (configmanager.PreviewResult, error) {
	c, e := b.clientFor(ctx, p, patch.ResourceID)
	if e != nil {
		return configmanager.PreviewResult{}, e
	}
	r, e := c.Preview(ctx, configActor(p), patch)
	if e == nil {
		b.mu.Lock()
		b.tokens[r.Token] = c
		b.mu.Unlock()
	}
	return r, e
}
func (b *ConfigBroker) Apply(ctx context.Context, p Principal, token string) (configmanager.ApplyResult, error) {
	b.mu.Lock()
	c := b.tokens[token]
	delete(b.tokens, token)
	b.mu.Unlock()
	if c == nil {
		return configmanager.ApplyResult{}, fmt.Errorf("proposal token is unknown to this broker")
	}
	return c.Apply(ctx, configActor(p), token)
}
func (b *ConfigBroker) Rollback(ctx context.Context, p Principal, id string) (configmanager.RollbackResult, error) {
	c, e := b.clientFor(ctx, p, id)
	if e != nil {
		return configmanager.RollbackResult{}, e
	}
	return c.Rollback(ctx, configActor(p), id)
}
