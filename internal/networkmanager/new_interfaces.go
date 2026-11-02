package networkmanager

import (
	"fmt"
	"net"
	"os"
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

func (s *Snapshot) managedSourceIndex() int {
	for index, file := range s.Files {
		if file.Path == managedInterfaces {
			return index
		}
	}
	return -1
}

func (s *Snapshot) includeUnconfigured(root string, p Policy, names map[string]string) error {
	unknown := []string{}
	for name := range names {
		if _, exists := s.View.Interfaces[name]; !exists && namePattern.MatchString(name) {
			unknown = append(unknown, name)
		}
	}
	runtimeManaged := false
	for _, file := range s.Files {
		runtimeManaged = runtimeManaged || file.Path == "run/netplan/"+filepath.Base(managedInterfaces)
	}
	if len(unknown) == 0 && !runtimeManaged {
		return nil
	}
	reason := ""
	for _, file := range s.Files {
		network := child(file.doc.Content[0], "network")
		if renderer := scalar(child(network, "renderer")); renderer != "" && renderer != "networkd" {
			reason = "Interface controlled by unsupported Netplan renderer (" + renderer + ")."
		}
		if file.Path == "run/netplan/"+filepath.Base(managedInterfaces) {
			reason = "new-interface source is shadowed by /run/netplan"
		}
		if file.Path == "lib/netplan/"+filepath.Base(managedInterfaces) {
			reason = "new-interface source filename conflicts with a vendor Netplan definition"
		}
	}
	for _, item := range s.View.Interfaces {
		if strings.Contains(item.ReadOnly, "match must resolve") {
			reason = "existing Netplan matches must resolve uniquely before adding interface definitions"
		}
	}
	if len(p.Files) > 0 && !contains(p.Files, filepath.Base(managedInterfaces)) {
		reason = "source filename 90-safeterm-interfaces.yaml is outside administrator policy"
	}
	index := s.managedSourceIndex()
	if index == -1 {
		file := File{Path: managedInterfaces, Data: []byte("# Managed by SafeTerm for newly discovered interfaces.\nnetwork:\n  version: 2\n  renderer: networkd\n  ethernets:\n"), Mode: 0600, UID: uint32(os.Geteuid()), GID: uint32(os.Getegid()), Attrs: map[string][]byte{}, Missing: true}
		// Use an explicit mapping while retaining block style for later patches.
		file.Data = append(file.Data, []byte("    # Interface definitions are added after a confirmed trial.\n    {}\n")...)
		if err := parseFile(&file); err != nil {
			return err
		}
		s.Files = append(s.Files, file)
	} else {
		lower := strings.ToLower(string(s.Files[index].Data))
		if !p.AllowGeneratedFiles && (strings.Contains(lower, "cloud-init") || strings.Contains(lower, "cloud_init")) {
			reason = "cloud-init generated file requires explicit administrator ownership acknowledgement"
		}
		network := child(s.Files[index].doc.Content[0], "network")
		if ether := child(network, "ethernets"); ether == nil || ether.Kind != yaml.MappingNode {
			reason = "new-interface source requires a Netplan ethernets mapping"
		}
	}
	for _, name := range unknown {
		item := Interface{Name: name, ID: name, Files: []string{managedInterfaces}, ReadOnly: reason}
		if !p.AllowsInterface(name) {
			item.ReadOnly = fmt.Sprintf("interface is outside administrator policy; allow %q or enable allow_all_interfaces in %s, then reopen config network interface", name, PolicyPath)
		}
		mac, valid := normalizedMAC(names[name])
		if valid {
			item.ID = "safeterm-" + strings.ReplaceAll(mac, ":", "")
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
	return nodeFor(value)
}
