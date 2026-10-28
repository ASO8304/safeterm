package broker

import (
	"fmt"
	"sort"
	"strings"
)

var networkActionDescriptions = map[string]string{
	"status":   "Show live addresses, routes and DNS for allowed interfaces",
	"diff":     "Compare the saved candidate with active Netplan configuration",
	"apply":    "Apply the saved candidate as a timed trial; confirmation is required",
	"confirm":  "Confirm your live trial and persist its Netplan changes",
	"rollback": "Reject the live trial, or restore the last confirmed network change",
	"discard":  "Discard your saved candidate; leave active networking unchanged",
}
var networkEditorDescriptions = map[string]string{
	"edit":     "Select an interface or named route to edit",
	"show":     "Show a compact interface table; edit NAME then show for addresses, DNS, MTU and source files",
	"diff":     "Show edits staged in this editor session",
	"set":      "Replace a field value or its entire list in the editor",
	"append":   "Add values to a list without removing existing entries",
	"unselect": "Remove specific values from a list",
	"unset":    "Clear a list, or remove the explicit MTU setting",
	"next":     nextDescription,
	"back":     backDescription,
	"save":     "Validate and commit the candidate, then exit; does not apply networking",
	"cancel":   "Exit and discard unsaved editor changes; keep any previously saved candidate",
	"delete":   "Remove a named route from the candidate",
}
var networkInterfaceFields = map[string]string{
	"addresses":      "set addresses <cidr>... - Replace all IP addresses (e.g., 10.0.0.1/24)",
	"dhcp4":          "Enable or disable DHCP for IPv4; static addresses are kept",
	"dns":            "DNS server IP addresses; multiple values allowed",
	"search-domains": "DNS search domains; multiple values allowed",
	"mtu":            "Explicit MTU in bytes (576..65535); unset removes the override",
}
var networkRouteFields = map[string]string{
	"interface":   "Allowed outgoing interface for this route",
	"destination": "Destination network CIDR, e.g. 10.50.0.0/16 or 0.0.0.0/0",
	"gateway":     "Gateway IP address in the same address family as the destination",
	"metric":      "Route priority from 0 to 4294967295; lower values are preferred",
}

func (s *Shell) networkHelpRow(name, description string) {
	s.writeTerminalData([]byte(fmt.Sprintf("%-24s %s\n", name, description)))
}
func (s *Shell) networkActionHelp(fields []string) error {
	if len(fields) == 0 {
		for _, name := range networkActions {
			s.networkHelpRow(name, networkActionDescriptions[name])
		}
		return nil
	}
	if len(fields) == 1 {
		if desc, ok := networkActionDescriptions[fields[0]]; ok {
			s.networkHelpRow("<cr>", desc)
			return nil
		}
	}
	return fmt.Errorf("usage: execute network ? or execute network ACTION ?")
}
func (s *Shell) networkContextHelp(kind string, selected bool) {
	values := []string{"?", "back", "edit", "show", "diff", "next", "save", "cancel", "exit", "clear", "whoami"}
	if kind == "route" {
		values = append(values, "delete")
	}
	if selected {
		values = append(values, "set")
		if kind == "interface" {
			values = append(values, "append", "unselect", "unset")
		}
	}
	sort.Strings(values)
	for _, name := range values {
		desc := networkEditorDescriptions[name]
		if global, ok := globalDescriptions[name]; ok {
			desc = global
		}
		if name == "?" {
			desc = "Show available commands; command ? keeps your input"
		}
		if name == "edit" {
			if kind == "interface" {
				desc = "Select an existing network interface"
			} else {
				desc = "Select an existing route or choose a name for a new route"
			}
		}
		if name == "show" && kind == "route" {
			desc = "Show candidate routes grouped by interface; limit to the selected route when editing"
		}
		s.networkHelpRow(name, desc)
	}
}

// Contextual help uses only the cached editor view. It never saves, applies,
// selects an item or sends a request to the privileged network manager.
func (s *Shell) networkEditorHelp(fields []string) error {
	e := s.networkEdit
	if len(fields) == 0 {
		s.networkContextHelp(e.kind, e.item != "")
		return nil
	}
	command := fields[0]
	switch command {
	case "edit", "delete":
		if command == "delete" && e.kind != "route" {
			return fmt.Errorf("delete is available only in the route context")
		}
		prefix := ""
		if len(fields) == 2 {
			prefix = fields[1]
		} else if len(fields) > 2 {
			return fmt.Errorf("usage: %s NAME", command)
		}
		names := []string{}
		if e.kind == "interface" {
			for name := range e.view.Interfaces {
				names = append(names, name)
			}
		} else {
			for name := range e.view.Routes {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			desc := "Select network interface"
			reason := ""
			if e.kind == "interface" {
				reason = e.view.Interfaces[name].ReadOnly
			} else {
				desc = "Select named route"
				reason = e.view.Routes[name].ReadOnly
			}
			if command == "delete" {
				desc = "Remove named route from the candidate"
			}
			if reason != "" {
				desc += "; read-only: " + reason
			}
			s.networkHelpRow(name, desc)
		}
		if e.kind == "route" && command == "edit" {
			s.networkHelpRow("<name>", "Choose a new route name; set interface, destination and optional gateway/metric")
		}
		return nil
	case "set", "append", "unselect", "unset":
		if e.item == "" {
			return fmt.Errorf("use edit NAME first")
		}
		if e.kind == "route" && command != "set" {
			return fmt.Errorf("routes use set FIELD VALUE; use delete NAME to remove a route")
		}
		definitions := networkInterfaceFields
		if e.kind == "route" {
			definitions = networkRouteFields
		}
		allowed := func(name string) bool {
			if command == "append" || command == "unselect" {
				return name == "addresses" || name == "dns" || name == "search-domains"
			}
			return command != "unset" || name != "dhcp4"
		}
		if len(fields) == 1 {
			names := []string{}
			for name := range definitions {
				if allowed(name) {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			for _, name := range names {
				s.networkHelpRow(name, definitions[name])
			}
			return nil
		}
		desc, ok := definitions[fields[1]]
		if !ok || !allowed(fields[1]) {
			return fmt.Errorf("unsupported field here; use %s ?", command)
		}
		if command == "unset" {
			if len(fields) != 2 {
				return fmt.Errorf("unset takes only a field name")
			}
			s.networkHelpRow("<cr>", "Clear this field in the editor; "+desc)
			return nil
		}
		if len(fields) > 2 {
			s.networkHelpRow("<cr>", "Stage the supplied value(s); validation is required before saving")
			return nil
		}
		if fields[1] == "dhcp4" {
			s.networkHelpRow("enable", "Enable IPv4 DHCP; keep configured static addresses")
			s.networkHelpRow("disable", "Disable IPv4 DHCP; keep configured static addresses")
			return nil
		}
		if e.kind == "route" && fields[1] == "interface" {
			names := []string{}
			for name, i := range e.view.Interfaces {
				if i.ReadOnly == "" {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			for _, name := range names {
				s.networkHelpRow(name, "Use this allowed outgoing interface")
			}
			return nil
		}
		placeholders := map[string]string{"addresses": "<cidr>...", "dns": "<IP> [...]", "search-domains": "<domain> [...]", "mtu": "<576..65535>", "destination": "<network/CIDR>", "gateway": "<IP>", "metric": "<0..4294967295>"}
		s.networkHelpRow(placeholders[fields[1]], desc)
		return nil
	default:
		desc, ok := networkEditorDescriptions[command]
		if !ok || len(fields) != 1 {
			return fmt.Errorf("unknown network editor command; use ?")
		}
		if command == "show" && e.kind == "route" {
			desc = "Show candidate routes grouped by interface; limit to the selected route when editing"
		}
		s.networkHelpRow("<cr>", desc)
		return nil
	}
}
