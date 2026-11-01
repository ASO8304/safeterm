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
	s.writeStyled(styleHeading, "Interfaces (configuration draft)\n\n")
	if len(e.view.Interfaces) == 0 {
		s.writeTerminalData([]byte("No supported interfaces found in Netplan configuration.\nUse exit to return to the network menu.\n"))
		return
	}
	names := make([]string, 0, len(e.view.Interfaces))
	for name := range e.view.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	var table bytes.Buffer
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tSTATE\tDHCP4\tADDRESSES\tEDITING")
	readOnly, changed := []string{}, false
	for _, name := range names {
		item := e.view.Interfaces[name]
		editing := "Allowed"
		if item.ReadOnly != "" {
			editing = "Read-only (" + interfaceRestrictionLabel(item.ReadOnly) + ")"
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
			addresses = []string{"None"}
		}
		state := e.view.States[name]
		if state == "" {
			state = "unknown"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", label, state, dhcp, addresses[0], editing)
		for _, address := range addresses[1:] {
			fmt.Fprintf(writer, "\t\t\t%s\t\n", address)
		}
	}
	_ = writer.Flush()
	s.writeTable(table.Bytes())
	if len(readOnly) > 0 {
		s.writeStyled(styleWarning, "\nRead-only details\n")
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
		s.writeStyled(styleWarning, "\n* Unsaved edits staged. Use diff to review; save commits the candidate.\n")
	} else {
		s.writeTerminalData([]byte("\nNo unsaved interface edits.\n"))
	}
	s.writeTerminalData([]byte("Details: edit NAME, then show.\nSTATE recorded at editor entry; use execute network status for live details.\n"))
}

func interfaceHasEdits(e *networkEditSession, name string) bool {
	return len(interfaceDiff(e.base.Interfaces[name], e.view.Interfaces[name])) > 0
}

func interfaceRestrictionLabel(reason string) string {
	switch {
	case strings.HasPrefix(reason, "interface is outside administrator policy"):
		return "Policy restricted"
	case strings.Contains(reason, "renderer"):
		return "Unsupported renderer"
	case strings.HasPrefix(reason, "legacy gateway"):
		return "Legacy gateway"
	case reason == "physical interface is not present":
		return "Device absent"
	default:
		return "Unsupported configuration"
	}
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
		s.writeTerminalData([]byte("Interface not found in this editor. Use exit to choose an interface.\n"))
		return
	}
	editing := "Editing Allowed"
	if item.ReadOnly != "" {
		editing = "Read-only (" + interfaceRestrictionLabel(item.ReadOnly) + ")"
	}
	s.writeStyled(styleHeading, fmt.Sprintf("Interface: %s (%s)\n", name, editing))
	if interfaceHasEdits(e, name) {
		s.writeStyled(styleWarning, "* Unsaved edits staged\n")
	}
	s.writeTerminalData([]byte("\n"))
	if item.ReadOnly != "" {
		reason, action := interfaceRestriction(item.ReadOnly)
		s.networkStyledParagraph(styleWarning, "", "Read-only: "+reason)
		if action != "" {
			s.networkParagraph("", "Action: "+action)
		}
		s.writeTerminalData([]byte("\n"))
	}
	changed := map[string]bool{}
	for _, field := range interfaceDiff(e.base.Interfaces[name], item) {
		changed[field.label] = true
	}
	row := func(label, empty string, values ...string) {
		if len(values) == 0 {
			values = []string{empty}
		}
		for i, value := range values {
			if changed[label] {
				value += " (staged)"
			}
			key := ""
			if i == 0 {
				key = label + ":"
			}
			s.writeStyled(styleCommand, fmt.Sprintf("%-16s", key))
			style := ""
			if label == "Source" {
				style = stylePath
			}
			if changed[label] {
				style = styleWarning
			}
			s.writeStyled(style, value+"\n")
		}
	}
	row("DHCP4", "", onOff(item.DHCP4))
	row("Addresses", "None", item.Addresses...)
	row("DNS Servers", "System default", item.DNS...)
	row("Search Domains", "System default", item.SearchDomains...)
	row("MTU", "", optionalNumber(item.MTU))
	if item.ID != "" && item.ID != name {
		row("Netplan ID", "", item.ID)
	}
	files := make([]string, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, "/"+strings.TrimPrefix(file, "/"))
	}
	row("Source", "Not available", files...)
}

// Wrap explanations independently of table columns so a policy denial does not
// turn the interface summary into a very wide terminal dump.
func (s *Shell) networkParagraph(indent, text string) {
	s.networkStyledParagraph("", indent, text)
}

func (s *Shell) networkStyledParagraph(style, indent, text string) {
	line := indent
	for _, word := range strings.Fields(text) {
		if len(line) > len(indent) && len(line)+1+len(word) > 78 {
			s.writeStyled(style, line+"\n")
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += word
	}
	if len(line) > len(indent) {
		s.writeStyled(style, line+"\n")
	}
}

// This is the configured candidate, not the kernel's DHCP/learned routing table.
// Display uses the cached editor view and never contacts or changes the network.
func (s *Shell) showNetworkRoutes(e *networkEditSession) {
	if e.item != "" {
		s.showNetworkRoute(e)
		return
	}
	s.writeStyled(styleHeading, "Routes (configuration draft)\n\n")
	if len(e.view.Routes) == 0 {
		s.writeTerminalData([]byte("No routes configured.\n"))
		return
	}
	var table bytes.Buffer
	writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tDESTINATION\tGATEWAY\tINTERFACE\tMETRIC\tDELETION")
	for _, id := range e.itemChoices() {
		name, _ := e.resolveItem(id)
		route := e.view.Routes[name]
		label := id
		for _, diff := range networkDiffs(e) {
			if diff.name == name {
				label += " *"
				break
			}
		}
		destination, gateway := route.Destination, route.Gateway
		if destination == "" {
			destination = "Not set"
		}
		if gateway == "" {
			gateway = "Direct"
		}
		deletion := "Allowed"
		if e.protectedRoute(name) {
			deletion = "Protected (default)"
		} else if route.ReadOnly != "" {
			deletion = "Read-only"
		}
		iface := route.Interface
		if iface == "" {
			iface = "Not set"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", label, destination, gateway, iface, optionalNumber(route.Metric), deletion)
	}
	_ = writer.Flush()
	s.writeTable(table.Bytes())
}
