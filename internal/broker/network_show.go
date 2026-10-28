package broker

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"safeterm/internal/networkmanager"
)

// Interface presentation stays scoped to the current context. Protocol fields
// such as versions, pending trials and route maps are not operator-facing data.
func (s *Shell) showNetworkInterfaces(e *networkEditSession) {
	if e.item != "" {
		s.showNetworkInterface(e, e.item)
		return
	}
	s.writeTerminalData([]byte("Interfaces (configuration draft)\n\n"))
	if len(e.view.Interfaces) == 0 {
		s.writeTerminalData([]byte("No supported interfaces found in Netplan configuration.\nUse back to return to the network menu.\n"))
		return
	}
	names := make([]string, 0, len(e.view.Interfaces))
	for name := range e.view.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	var table bytes.Buffer
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tDHCP4\tADDRESSES\tEDITING")
	readOnly, changed := []string{}, false
	for _, name := range names {
		item := e.view.Interfaces[name]
		editing := "Allowed"
		if item.ReadOnly != "" {
			editing = "Read-only"
			readOnly = append(readOnly, name)
		}
		label := name
		if interfaceHasEdits(e, name) {
			label, changed = name+" *", true
		}
		dhcp := "off"
		if item.DHCP4 {
			dhcp = "on"
		}
		addresses := item.Addresses
		if len(addresses) == 0 {
			addresses = []string{"none"}
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", label, dhcp, addresses[0], editing)
		for _, address := range addresses[1:] {
			fmt.Fprintf(writer, "\t\t%s\t\n", address)
		}
	}
	_ = writer.Flush()
	s.writeTerminalData(table.Bytes())
	if len(readOnly) > 0 {
		s.writeTerminalData([]byte("\nRead-only details\n"))
		policyBlocked := false
		for _, name := range readOnly {
			reason, _ := interfaceRestriction(e.view.Interfaces[name].ReadOnly)
			s.networkParagraph("  ", name+": "+reason)
			policyBlocked = policyBlocked || strings.HasPrefix(e.view.Interfaces[name].ReadOnly, "interface is outside administrator policy")
		}
		if policyBlocked {
			s.networkParagraph("  ", "Policy: "+networkmanager.PolicyPath)
			s.networkParagraph("  ", "A Linux administrator must allow these interfaces, then leave and reopen the network editor.")
		}
	}
	if changed {
		s.writeTerminalData([]byte("\n* Unsaved edits. Use diff to review; end saves the candidate.\n"))
	} else {
		s.writeTerminalData([]byte("\nNo unsaved interface edits.\n"))
	}
	s.writeTerminalData([]byte("Details: edit NAME, then show.\nFor live state, leave the editor and use execute network status.\nCommands: edit NAME | diff | back | ?\n"))
}

func interfaceHasEdits(e *networkEditSession, name string) bool {
	for _, change := range e.changes {
		if change.Kind == "interface" && change.Name == name {
			return true
		}
	}
	return false
}

func interfaceRestriction(reason string) (string, string) {
	switch {
	case strings.HasPrefix(reason, "interface is outside administrator policy"):
		return "Not allowed by administrator policy.", "Add this interface to interfaces in " + networkmanager.PolicyPath + ", then leave and reopen the network editor."
	case reason == "legacy gateway4/gateway6 must be migrated to routes by the administrator":
		return "Legacy gateway4/gateway6 configuration.", "A Linux administrator must convert the existing gateway to a Netplan default route before editing."
	case reason == "physical interface is not present":
		return "Device is not present.", "Check the interface name and host network configuration."
	default:
		return reason, ""
	}
}

func (s *Shell) showNetworkInterface(e *networkEditSession, name string) {
	item, ok := e.view.Interfaces[name]
	if !ok {
		s.writeTerminalData([]byte("Interface not found in this editor. Use back to choose an interface.\n"))
		return
	}
	s.writeTerminalData([]byte(fmt.Sprintf("Interface %s (configuration draft)\n\n", name)))
	row := func(label string, values ...string) {
		if len(values) == 0 {
			values = []string{"not configured"}
		}
		for i, value := range values {
			if i == 0 {
				s.writeTerminalData([]byte(fmt.Sprintf("  %-16s : %s\n", label, value)))
			} else {
				s.writeTerminalData([]byte(fmt.Sprintf("  %-16s   %s\n", "", value)))
			}
		}
	}
	editing := "Allowed"
	if item.ReadOnly != "" {
		editing = "Read-only"
	}
	row("Editing", editing)
	if item.ReadOnly != "" {
		reason, action := interfaceRestriction(item.ReadOnly)
		s.networkParagraph("  ", "Reason: "+reason)
		if action != "" {
			s.networkParagraph("  ", "Action: "+action)
		}
		s.writeTerminalData([]byte("\n"))
	}
	dhcp := "disabled"
	if item.DHCP4 {
		dhcp = "enabled"
	}
	row("IPv4 DHCP", dhcp)
	row("Addresses", item.Addresses...)
	row("DNS servers", item.DNS...)
	row("Search domains", item.SearchDomains...)
	mtu := "automatic"
	if item.MTU != nil {
		mtu = fmt.Sprint(*item.MTU)
	}
	row("MTU", mtu)
	if item.ID != "" && item.ID != name {
		row("Netplan ID", item.ID)
	}
	files := make([]string, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, "/"+strings.TrimPrefix(file, "/"))
	}
	row("Source files", files...)
	changes := "none"
	if interfaceHasEdits(e, name) {
		changes = "unsaved; use diff to review"
	}
	row("Changes", changes)
	s.writeTerminalData([]byte("\nFor live state, leave the editor and use execute network status.\n"))
	if item.ReadOnly == "" {
		s.writeTerminalData([]byte("Commands: set FIELD VALUE | diff | back | ?\n"))
	} else {
		s.writeTerminalData([]byte("Commands: back | ?\n"))
	}
}

// Wrap explanations independently of table columns so a policy denial does not
// turn the interface summary into a very wide terminal dump.
func (s *Shell) networkParagraph(indent, text string) {
	line := indent
	for _, word := range strings.Fields(text) {
		if len(line) > len(indent) && len(line)+1+len(word) > 78 {
			s.writeTerminalData([]byte(line + "\n"))
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += word
	}
	if len(line) > len(indent) {
		s.writeTerminalData([]byte(line + "\n"))
	}
}

// This is the configured candidate, not the kernel's DHCP/learned routing table.
// Display uses the cached editor view and never contacts or changes the network.
func (s *Shell) showNetworkRoutes(e *networkEditSession) {
	groups := map[string][]networkmanager.Route{}
	if e.item == "" {
		for name := range e.view.Interfaces {
			groups[name] = nil
		}
	}
	if e.item != "" {
		if _, exists := e.view.Routes[e.item]; !exists {
			s.writeTerminalData([]byte(fmt.Sprintf("Route %s has no fields yet. Use set interface NAME and set destination NETWORK/CIDR; gateway and metric are optional.\n", e.item)))
			return
		}
	}
	for name, route := range e.view.Routes {
		if e.item != "" && name != e.item {
			continue
		}
		route.Name = name
		groups[route.Interface] = append(groups[route.Interface], route)
	}
	s.writeTerminalData([]byte("Configured routes (candidate)\n"))
	if len(groups) == 0 {
		s.writeTerminalData([]byte("No configured interfaces or routes.\n"))
		return
	}
	names := []string{}
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		heading := "Interface " + name
		if name == "" {
			heading = "Unassigned routes (use set interface NAME)"
		}
		if item, exists := e.view.Interfaces[name]; exists && item.ReadOnly != "" {
			heading += " [read-only: " + item.ReadOnly + "]"
		}
		s.writeTerminalData([]byte(heading + "\n"))
		routes := groups[name]
		sort.Slice(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
		if len(routes) == 0 {
			s.writeTerminalData([]byte("  No entries in the Netplan routes list.\n"))
			continue
		}
		var table bytes.Buffer
		writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "  NAME\tDESTINATION\tGATEWAY\tMETRIC")
		for _, route := range routes {
			destination := route.Destination
			if destination == "" {
				destination = "<not set>"
			}
			gateway := route.Gateway
			if gateway == "" {
				gateway = "-"
			}
			metric := "auto"
			if route.Metric != nil {
				metric = fmt.Sprint(*route.Metric)
			}
			label := route.Name
			if route.ReadOnly != "" {
				label += " [read-only]"
			}
			fmt.Fprintf(writer, "  %s\t%s\t%s\t%s\n", label, destination, gateway, metric)
		}
		_ = writer.Flush()
		s.writeTerminalData(table.Bytes())
		total += len(routes)
	}
	if total == 0 {
		s.writeTerminalData([]byte("Use edit NAME to create a route, then set interface NAME and set destination NETWORK/CIDR.\n"))
	}
}
