// Package networkmanager is the typed, restricted Netplan boundary. It never
// accepts arbitrary paths, programs, YAML or command arguments from a terminal.
package networkmanager

import (
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

const Socket = "/run/safeterm-network-manager/manager.sock"
const PolicyPath = "/etc/safeterm/network/policy.json"
const StatePath = "/var/lib/safeterm/network"
const maxFiles = 64
const maxBytes = 4 << 20

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var domainPattern = regexp.MustCompile(`^~?[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)

type Policy struct {
	Version             int      `json:"version"`
	Interfaces          []string `json:"interfaces"`
	Files               []string `json:"files,omitempty"` // basenames; empty discovers all /etc files
	ReadRoles           []string `json:"read_roles"`
	EditRoles           []string `json:"edit_roles"`
	ConfirmSeconds      int      `json:"confirm_seconds"`
	AllowGeneratedFiles bool     `json:"allow_generated_files"`
}

func DefaultPolicy() Policy {
	return Policy{Version: 1, Interfaces: []string{}, ReadRoles: []string{"netops", "network-viewer", "network-operator"}, EditRoles: []string{"netops", "network-operator"}, ConfirmSeconds: 120}
}
func (p Policy) Validate() error {
	if p.Version != 1 || p.ConfirmSeconds < 10 || p.ConfirmSeconds > 300 || len(p.Interfaces) > 64 || len(p.Files) > 64 || len(p.ReadRoles) == 0 || len(p.EditRoles) == 0 {
		return fmt.Errorf("network: invalid policy version, limits, roles or confirmation timeout (10..300)")
	}
	for _, items := range [][]string{p.Interfaces, p.Files, p.ReadRoles, p.EditRoles} {
		seen := map[string]bool{}
		for _, item := range items {
			if !namePattern.MatchString(item) || seen[item] {
				return fmt.Errorf("network: invalid or duplicate policy name %q", item)
			}
			seen[item] = true
		}
	}
	for _, file := range p.Files {
		if !strings.HasSuffix(file, ".yaml") {
			return fmt.Errorf("network: policy files must be exact .yaml basenames")
		}
	}
	return nil
}

type Interface struct {
	Name          string   `json:"name"`
	ID            string   `json:"id"`
	DHCP4         bool     `json:"dhcp4"`
	Addresses     []string `json:"addresses"`
	DNS           []string `json:"dns"`
	SearchDomains []string `json:"search_domains"`
	MTU           *int     `json:"mtu,omitempty"`
	Files         []string `json:"files"`
	ReadOnly      string   `json:"read_only,omitempty"`
}
type Route struct {
	Name        string  `json:"name"`
	Interface   string  `json:"interface"`
	Destination string  `json:"destination"`
	Gateway     string  `json:"gateway,omitempty"`
	Metric      *uint32 `json:"metric,omitempty"`
	ReadOnly    string  `json:"read_only,omitempty"`
}
type View struct {
	Version string `json:"version"`
	// Link states are display metadata, excluded from configuration identity.
	States     map[string]string    `json:"states,omitempty"`
	Interfaces map[string]Interface `json:"interfaces"`
	Routes     map[string]Route     `json:"routes"`
	Pending    bool                 `json:"pending"`
	Deadline   string               `json:"deadline,omitempty"`
	Owner      string               `json:"owner,omitempty"`
}
type Change struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Field  string   `json:"field,omitempty"`
	Action string   `json:"action"`
	Values []string `json:"values,omitempty"`
}
type Request struct {
	Action  string   `json:"action"`
	Version string   `json:"version,omitempty"`
	Changes []Change `json:"changes,omitempty"`
}
type Reply struct {
	View *View  `json:"view,omitempty"`
	Text string `json:"text,omitempty"`
}

func Modify(v View, changes []Change) (View, error) {
	v = cloneView(v)
	if len(changes) > 256 {
		return v, fmt.Errorf("network: too many changes")
	}
	for _, c := range changes {
		if !namePattern.MatchString(c.Name) || len(c.Values) > 64 {
			return v, fmt.Errorf("network: invalid item name or excessive values")
		}
		switch c.Kind {
		case "interface":
			item, ok := v.Interfaces[c.Name]
			if !ok {
				return v, fmt.Errorf("network: unknown interface %q", c.Name)
			}
			if item.ReadOnly != "" {
				return v, fmt.Errorf("network: interface %s: %s", c.Name, item.ReadOnly)
			}
			if c.Action != "set" && c.Action != "append" && c.Action != "unselect" && c.Action != "unset" {
				return v, fmt.Errorf("network: invalid interface edit")
			}
			var target *[]string
			switch c.Field {
			case "addresses":
				for _, value := range c.Values {
					if err := validateAddress(value); err != nil {
						return v, err
					}
				}
				target = &item.Addresses
			case "dns":
				target = &item.DNS
			case "search-domains":
				target = &item.SearchDomains
			case "dhcp4":
				if c.Action != "set" || len(c.Values) != 1 || (c.Values[0] != "enable" && c.Values[0] != "disable") {
					return v, fmt.Errorf("network: set dhcp4 enable|disable")
				}
				item.DHCP4 = c.Values[0] == "enable"
			case "mtu":
				if c.Action == "unset" && len(c.Values) == 0 {
					item.MTU = nil
					break
				}
				if c.Action != "set" || len(c.Values) != 1 {
					return v, fmt.Errorf("network: set mtu INTEGER")
				}
				var n int
				if _, err := fmt.Sscanf(c.Values[0], "%d", &n); err != nil || fmt.Sprint(n) != c.Values[0] || n < 576 || n > 65535 {
					return v, fmt.Errorf("network: mtu must be 576..65535")
				}
				item.MTU = &n
			default:
				return v, fmt.Errorf("network: unknown interface field %q", c.Field)
			}
			if target != nil {
				if c.Action == "unset" {
					if len(c.Values) != 0 {
						return v, fmt.Errorf("network: unset takes no values")
					}
					*target = nil
				} else {
					if len(c.Values) == 0 {
						return v, fmt.Errorf("network: missing list values")
					}
					if c.Action == "set" {
						*target = append([]string(nil), c.Values...)
					} else {
						for _, value := range c.Values {
							if c.Action == "append" {
								if !slices.Contains(*target, value) {
									*target = append(*target, value)
								}
							} else {
								*target = slices.DeleteFunc(*target, func(s string) bool { return s == value })
							}
						}
					}
				}
			}
			v.Interfaces[c.Name] = item
		case "route":
			r, exists := v.Routes[c.Name]
			if exists && r.ReadOnly != "" {
				return v, fmt.Errorf("network: route is read-only: %s", r.ReadOnly)
			}
			if c.Action == "delete" {
				if !exists {
					return v, fmt.Errorf("network: unknown route")
				}
				delete(v.Routes, c.Name)
				continue
			}
			if c.Action != "set" || len(c.Values) != 1 {
				return v, fmt.Errorf("network: route requires set FIELD VALUE")
			}
			r.Name = c.Name
			switch c.Field {
			case "interface":
				r.Interface = c.Values[0]
			case "destination":
				r.Destination = c.Values[0]
			case "gateway":
				r.Gateway = c.Values[0]
			case "metric":
				var n uint64
				if _, err := fmt.Sscanf(c.Values[0], "%d", &n); err != nil || fmt.Sprint(n) != c.Values[0] || n > 4294967295 {
					return v, fmt.Errorf("network: invalid route metric")
				}
				m := uint32(n)
				r.Metric = &m
			default:
				return v, fmt.Errorf("network: unknown route field")
			}
			v.Routes[c.Name] = r
		default:
			return v, fmt.Errorf("network: unknown edit kind")
		}
	}
	return v, nil
}
func ValidateView(v View) error {
	if len(v.Interfaces) > 64 || len(v.Routes) > 256 {
		return fmt.Errorf("network: excessive interfaces/routes")
	}
	for _, i := range v.Interfaces {
		if len(i.Addresses) > 64 || len(i.DNS) > 16 || len(i.SearchDomains) > 16 {
			return fmt.Errorf("network: excessive interface values")
		}
		seen := map[string]bool{}
		for _, a := range i.Addresses {
			if err := validateAddress(a); err != nil {
				return err
			}
			if seen[a] {
				return fmt.Errorf("network: interface %s invalid or duplicate address %q", i.Name, a)
			}
			seen[a] = true
		}
		for _, a := range i.DNS {
			ip, err := netip.ParseAddr(a)
			if err != nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" {
				return fmt.Errorf("network: interface %s invalid DNS %q", i.Name, a)
			}
		}
		for _, d := range i.SearchDomains {
			if !domainPattern.MatchString(d) {
				return fmt.Errorf("network: invalid search domain %q", d)
			}
		}
	}
	seen := map[string]bool{}
	for _, r := range v.Routes {
		if r.ReadOnly != "" {
			continue
		}
		i, ok := v.Interfaces[r.Interface]
		if !ok || i.ReadOnly != "" {
			return fmt.Errorf("network: route %s requires an editable interface", r.Name)
		}
		p, err := netip.ParsePrefix(r.Destination)
		if err != nil || p != p.Masked() {
			return fmt.Errorf("network: route %s needs a network destination CIDR", r.Name)
		}
		if r.Gateway != "" {
			g, err := netip.ParseAddr(r.Gateway)
			if err != nil || g.IsUnspecified() || g.IsMulticast() || g.Zone() != "" || g.Is4() != p.Addr().Is4() {
				return fmt.Errorf("network: route %s has invalid gateway", r.Name)
			}
		}
		key := fmt.Sprintf("%s|%s|%s|%v", r.Interface, r.Destination, r.Gateway, metricValue(r.Metric))
		if seen[key] {
			return fmt.Errorf("network: duplicate route %s", r.Name)
		}
		seen[key] = true
	}
	return nil
}

func validateAddress(value string) error {
	if !strings.Contains(value, "/") {
		return fmt.Errorf("'%s' is missing a CIDR prefix. Format as Address/Prefix (e.g., 10.50.0.0/16).", value)
	}
	p, err := netip.ParsePrefix(value)
	if err != nil || p.Addr().IsUnspecified() || p.Addr().IsMulticast() || p.Addr().Zone() != "" {
		return fmt.Errorf("invalid interface address %q; use Address/Prefix (e.g., 10.0.0.1/24)", value)
	}
	return nil
}
func metricValue(p *uint32) string {
	if p == nil {
		return "auto"
	}
	return fmt.Sprint(*p)
}
func cloneView(v View) View {
	result := v
	result.States = maps.Clone(v.States)
	result.Interfaces = map[string]Interface{}
	result.Routes = map[string]Route{}
	for k, i := range v.Interfaces {
		i.Addresses = append([]string(nil), i.Addresses...)
		i.DNS = append([]string(nil), i.DNS...)
		i.SearchDomains = append([]string(nil), i.SearchDomains...)
		i.Files = append([]string(nil), i.Files...)
		if i.MTU != nil {
			m := *i.MTU
			i.MTU = &m
		}
		result.Interfaces[k] = i
	}
	for k, r := range v.Routes {
		if r.Metric != nil {
			m := *r.Metric
			r.Metric = &m
		}
		result.Routes[k] = r
	}
	return result
}
