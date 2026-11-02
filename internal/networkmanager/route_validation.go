package networkmanager

import (
	"fmt"
	"net/netip"
	"strings"
)

// RouteValidationError identifies the actual invalid item independently of the
// selected editor. The terminal can map Name to its local display ID.
type RouteValidationError struct {
	Route   Route
	Field   string
	Problem string
}

func (e *RouteValidationError) Error() string {
	label := "route to " + e.Route.Destination
	if IsDefaultRoute(e.Route) {
		label = "default IPv4 route"
		if e.Route.Destination == "::/0" {
			label = "default IPv6 route"
		}
	}
	if e.Route.Interface != "" {
		label += " on interface " + e.Route.Interface
	}
	return "network: " + label + ": " + e.Problem
}

func routeIssue(r Route, field, problem string) error {
	return &RouteValidationError{Route: r, Field: field, Problem: problem}
}

func routeDestinationProblem(value string) string {
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return fmt.Sprintf("destination %q must be a network CIDR, such as 10.0.0.0/8 or 0.0.0.0/0", value)
	}
	if prefix != prefix.Masked() {
		return fmt.Sprintf("destination %q contains host bits; use the network %s", value, prefix.Masked())
	}
	return ""
}

func routeGatewayProblem(value string) string {
	if strings.Contains(value, "/") {
		hint := "e.g. set gateway 192.0.2.1"
		if prefix, err := netip.ParsePrefix(value); err == nil && !prefix.Addr().Unmap().IsUnspecified() && !prefix.Addr().Unmap().IsMulticast() {
			hint = "use set gateway " + prefix.Addr().String()
		}
		return fmt.Sprintf("gateway %q must be an IP address without a CIDR prefix; %s", value, hint)
	}
	address, err := netip.ParseAddr(value)
	if err != nil || address.Unmap().IsUnspecified() || address.Unmap().IsMulticast() || address.Zone() != "" {
		return fmt.Sprintf("gateway %q must be a unicast IP address without a CIDR prefix", value)
	}
	return ""
}

func routeInterfaceProblem(v View, name string) string {
	if item, ok := v.Interfaces[name]; ok && item.ReadOnly == "" {
		return ""
	}
	for current, item := range v.Interfaces {
		if name != "" && item.PersistentName == name && current != name {
			return fmt.Sprintf("interface %q is a future persistent name; use the current name %q until reboot/device activation", name, current)
		}
	}
	return fmt.Sprintf("interface %q is not an editable current interface; use set interface ? to list choices", name)
}

// ValidateRoute checks complete routes, including imported configuration. Partial
// drafts validate only the field being entered; cross-field checks run at save.
func ValidateRoute(v View, r Route) error {
	if r.ReadOnly != "" {
		return nil
	}
	if problem := routeInterfaceProblem(v, r.Interface); problem != "" {
		return routeIssue(r, "interface", problem)
	}
	if problem := routeDestinationProblem(r.Destination); problem != "" {
		return routeIssue(r, "destination", problem)
	}
	if r.Gateway != "" {
		if problem := routeGatewayProblem(r.Gateway); problem != "" {
			return routeIssue(r, "gateway", problem)
		}
		gateway, _ := netip.ParseAddr(r.Gateway)
		destination, _ := netip.ParsePrefix(r.Destination)
		if gateway.Is4() != destination.Addr().Is4() {
			return routeIssue(r, "gateway", fmt.Sprintf("gateway %q and destination %q must use the same IP address family", r.Gateway, r.Destination))
		}
	}
	return nil
}
