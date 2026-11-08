package networkmanager

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const managedInterfaces = "etc/netplan/90-safeterm-interfaces.yaml"

func normalizedMAC(value string) (string, bool) {
	mac, err := net.ParseMAC(value)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return "", false
	}
	return mac.String(), true
}

func (s *Snapshot) sourceIndex(path string) int {
	for index, file := range s.Files {
		if file.Path == path {
			return index
		}
	}
	return -1
}

// New hardware joins an existing administrator-owned source. Never invent a
// second Netplan file; ambiguous destinations require an explicit policy choice.
func (s *Snapshot) newInterfaceDestination(p Policy) (string, string) {
	candidates := []string{}
	reasons := []string{}
	for _, file := range s.Files {
		if filepath.Dir(file.Path) != "etc/netplan" {
			continue
		}
		base := filepath.Base(file.Path)
		if p.NewInterfaceFile != "" && base != p.NewInterfaceFile {
			continue
		}
		reason := ""
		network := child(file.doc.Content[0], "network")
		if r := scalar(child(network, "renderer")); r != "" && r != "networkd" {
			reason = "Interface controlled by unsupported Netplan renderer (" + r + ")."
		}
		if s.sourceIndex("run/netplan/"+base) >= 0 {
			reason = "new-interface source is shadowed by /run/netplan"
		}
		if len(p.Files) > 0 && !contains(p.Files, base) {
			reason = "source filename " + base + " is outside administrator policy"
		}
		lower := strings.ToLower(string(file.Data))
		if !p.AllowGeneratedFiles && (strings.Contains(lower, "cloud-init") || strings.Contains(lower, "cloud_init")) {
			reason = "cloud-init generated file requires explicit administrator ownership acknowledgement"
		}
		if ether := child(network, "ethernets"); ether != nil && ether.Kind != yaml.MappingNode {
			reason = "new-interface source requires a Netplan ethernets mapping"
		}
		if reason != "" {
			reasons = append(reasons, reason)
			continue
		}
		candidates = append(candidates, file.Path)
	}
	if p.NewInterfaceFile == "" && len(candidates) > 1 {
		primary := []string{}
		for _, path := range candidates {
			if path != managedInterfaces {
				primary = append(primary, path)
			}
		}
		if len(primary) > 0 {
			candidates = primary
		}
	}
	if len(candidates) == 1 {
		return candidates[0], ""
	}
	if len(candidates) > 1 {
		return "", "multiple eligible Netplan files; a Linux administrator must select new_interface_file in " + PolicyPath
	}
	if len(reasons) == 1 {
		return "", reasons[0]
	}
	return "", "no eligible existing Netplan file; a Linux administrator must select an existing new_interface_file in " + PolicyPath
}

func (s *Snapshot) includeUnconfigured(root string, p Policy, names map[string]string) error {
	unknown := []string{}
	for name := range names {
		if _, exists := s.View.Interfaces[name]; !exists && namePattern.MatchString(name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	destination, reason := s.newInterfaceDestination(p)
	if s.unresolvedMatches {
		reason = "existing Netplan matches must resolve uniquely before adding interface definitions"
	}
	for _, file := range s.Files {
		if renderer := scalar(child(child(file.doc.Content[0], "network"), "renderer")); renderer != "" && renderer != "networkd" {
			reason = "Interface controlled by unsupported Netplan renderer (" + renderer + ")."
		}
	}
	for _, item := range s.View.Interfaces {
		if strings.Contains(item.ReadOnly, "match must resolve") {
			reason = "existing Netplan matches must resolve uniquely before adding interface definitions"
		}
	}
	for _, name := range unknown {
		item := Interface{Name: name, ID: name, ReadOnly: reason}
		if destination != "" {
			item.Files = []string{destination}
		}
		if !p.AllowsInterface(name) {
			item.ReadOnly = fmt.Sprintf("interface is outside administrator policy; allow %q or enable allow_all_interfaces in %s, then reopen config network interface", name, PolicyPath)
		}
		mac, valid := normalizedMAC(names[name])
		if valid {
			item.ID = "safeterm-" + strings.ReplaceAll(mac, ":", "")
			if s.definitionIDs[item.ID] {
				item.ReadOnly = "generated Netplan ID conflicts with an existing definition"
			}
			for _, existing := range s.View.Interfaces {
				if existing.ID == item.ID {
					item.ReadOnly = "generated Netplan ID conflicts with an existing definition"
				}
			}
		}
		if !valid {
			item.ReadOnly = "new interface requires a valid Ethernet MAC address"
		}
		for other, otherMAC := range names {
			if other != name && strings.EqualFold(mac, otherMAC) {
				item.ReadOnly = "MAC address is shared by multiple present interfaces"
			}
		}
		s.View.Interfaces[name] = item
	}
	return nil
}

func interfaceNode(item Interface, mac string, routes []Route) *yaml.Node {
	value := map[string]any{"match": map[string]string{"macaddress": mac}, "dhcp4": item.DHCP4}
	if len(item.Addresses) > 0 {
		value["addresses"] = item.Addresses
	}
	if item.MTU != nil {
		value["mtu"] = *item.MTU
	}
	if len(item.DNS) > 0 || len(item.SearchDomains) > 0 {
		value["nameservers"] = map[string]any{"addresses": nonNil(item.DNS), "search": nonNil(item.SearchDomains)}
	}
	if len(routes) > 0 {
		entries := []map[string]any{}
		for _, route := range routes {
			entry := map[string]any{"to": route.Destination}
			if route.Gateway != "" {
				entry["via"] = route.Gateway
			}
			if route.Metric != nil {
				entry["metric"] = *route.Metric
			}
			entries = append(entries, entry)
		}
		value["routes"] = entries
	}
	node := nodeFor(value)
	blockYAML(node)
	return node
}
