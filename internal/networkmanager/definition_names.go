package networkmanager

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

func simplePhysicalDefinition(fragments []fragment) bool {
	for _, f := range fragments {
		if n := child(f.node, "set-name"); n != nil && !interfaceNamePattern.MatchString(n.Value) {
			return false
		}
		if n := child(f.node, "match"); n != nil {
			if n.Kind != yaml.MappingNode || len(n.Content) == 0 {
				return false
			}
			for i := 0; i < len(n.Content); i += 2 {
				key, value := n.Content[i].Value, n.Content[i+1].Value
				switch key {
				case "name":
					if !interfaceNamePattern.MatchString(value) {
						return false
					}
				case "macaddress":
					if _, valid := normalizedMAC(value); !valid {
						return false
					}
				default:
					return false
				}
			}
		}
	}
	return true
}

// A pending Netplan set-name can make discovery miss a still-present MAC.
// Such a definition must never be eligible for absent-hardware deletion.
func definitionHasPresentHardware(id string, fragments []fragment, live map[string]string) bool {
	hasMAC := false
	for _, f := range fragments {
		mac := scalar(child(child(f.node, "match"), "macaddress"))
		if mac != "" {
			hasMAC = true
			for _, present := range live {
				if strings.EqualFold(mac, present) {
					return true
				}
			}
		}
	}
	if hasMAC {
		return false
	}
	if _, present := live[id]; present {
		return true
	}
	for _, f := range fragments {
		for _, name := range []string{scalar(child(child(f.node, "match"), "name")), scalar(child(f.node, "set-name"))} {
			if _, present := live[name]; present {
				return true
			}
		}
	}
	return false
}

// Keep definition IDs readable while MAC matching keeps the configuration bound
// to the same hardware before and after the deferred .link name takes effect.
// This operates after field rendering to avoid overlapping text patches.
func (s *Snapshot) reconcileDefinitions(view View, changed map[string][]byte, named []string) (map[string][]byte, error) {
	working := *s
	working.Files = append([]File(nil), s.Files...)
	for i := range working.Files {
		if data, ok := changed[working.Files[i].Path]; ok {
			working.Files[i].Data = data
			if err := parseFile(&working.Files[i]); err != nil {
				return nil, err
			}
		}
	}
	removed := []string{}
	for id, item := range s.View.AbsentInterfaces {
		if _, keep := view.AbsentInterfaces[id]; !keep {
			if item.ReadOnly != "" {
				return nil, fmt.Errorf("network: cannot delete %s: %s", id, item.ReadOnly)
			}
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	for _, id := range removed {
		if working.referencesDefinition(id) {
			return nil, fmt.Errorf("network: Netplan ID %q is referenced by other configuration; a Linux administrator must update those references before deletion", id)
		}
		for _, f := range s.absentFragments[id] {
			file := &working.Files[f.file]
			if err := normalizeDefinitionMapping(file); err != nil {
				return nil, err
			}
			ether := child(child(file.doc.Content[0], "network"), "ethernets")
			patch, err := fieldPatch(*file, ether, id, nil)
			if err != nil {
				return nil, err
			}
			file.Data = append(append(append([]byte(nil), file.Data[:patch.start]...), patch.data...), file.Data[patch.end:]...)
			// An empty ethernets mapping must remain a mapping, rather than YAML null.
			if err := parseFile(file); err != nil {
				return nil, err
			}
			ether = child(child(file.doc.Content[0], "network"), "ethernets")
			if ether != nil && ether.Tag == "!!null" {
				network := child(file.doc.Content[0], "network")
				patch, err = fieldPatch(*file, network, "ethernets", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
				if err != nil {
					return nil, err
				}
				file.Data = append(append(append([]byte(nil), file.Data[:patch.start]...), patch.data...), file.Data[patch.end:]...)
				if err := parseFile(file); err != nil {
					return nil, err
				}
			}
			changed[file.Path] = file.Data
		}
	}
	names := []string{}
	for name := range view.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		item := view.Interfaces[name]
		old := s.View.Interfaces[name]
		if item.ReadOnly != "" || !contains(named, name) && equal(old, item) && equal(routesFor(s.View, name), routesFor(view, name)) {
			continue
		}
		desired := item.PersistentName
		unconfigured := len(s.fragments[name]) == 0
		if desired == "" && unconfigured {
			desired = name
		}
		if desired == "" || desired == item.ID && !unconfigured {
			continue
		}
		if !namePattern.MatchString(desired) || desired == "lo" {
			return nil, fmt.Errorf("network: invalid persistent Netplan ID %q", desired)
		}
		for _, file := range working.Files {
			network := child(file.doc.Content[0], "network")
			for i := 0; i < len(network.Content); i += 2 {
				if child(network.Content[i+1], desired) != nil {
					if desired == item.ID && network.Content[i].Value == "ethernets" && contains(old.Files, file.Path) {
						continue // the new definition just inserted by render
					}
					if _, retained := view.AbsentInterfaces[desired]; retained {
						return nil, fmt.Errorf("network: Netplan ID %q already exists; use show absent and explicitly delete the retained definition first, or choose another name", desired)
					}
					return nil, fmt.Errorf("network: Netplan ID %q already exists in %s in /%s; choose another name or ask a Linux administrator to resolve the conflict", desired, network.Content[i].Value, file.Path)
				}
			}
		}
		if desired == item.ID {
			continue
		}
		if working.referencesDefinition(item.ID) {
			return nil, fmt.Errorf("network: Netplan ID %q is referenced by other configuration; a Linux administrator must update those references before renaming", item.ID)
		}
		paths := old.Files
		for position, path := range paths {
			index := working.sourceIndex(path)
			if index < 0 {
				return nil, fmt.Errorf("network: missing naming source")
			}
			file := &working.Files[index]
			if err := normalizeDefinitionMapping(file); err != nil {
				return nil, err
			}
			// A previously requested .link name may have been saved by an older
			// release without MAC-binding Netplan. Bind before changing its key.
			for _, field := range []string{"match", "set-name"} {
				ether := child(child(file.doc.Content[0], "network"), "ethernets")
				node := child(ether, item.ID)
				if node == nil {
					return nil, fmt.Errorf("network: missing interface naming fragment")
				}
				var value *yaml.Node
				if field == "match" && position == len(paths)-1 {
					mac, valid := normalizedMAC(s.View.MACs[name])
					if !valid {
						return nil, fmt.Errorf("network: persistent naming requires a valid Ethernet MAC address")
					}
					value = nodeFor(map[string]string{"macaddress": mac})
					current := child(node, field)
					if current != nil && len(current.Content) == 2 && scalar(current.Content[0]) == "macaddress" && strings.EqualFold(scalar(current.Content[1]), mac) {
						continue
					}
				} else if child(node, field) == nil {
					continue
				}
				patch, err := fieldPatch(*file, node, field, value)
				if err != nil {
					return nil, err
				}
				file.Data = append(append(append([]byte(nil), file.Data[:patch.start]...), patch.data...), file.Data[patch.end:]...)
				if err := parseFile(file); err != nil {
					return nil, err
				}
			}
			ether := child(child(file.doc.Content[0], "network"), "ethernets")
			if ether == nil {
				return nil, fmt.Errorf("network: missing Ethernet mapping")
			}
			for i := 0; i < len(ether.Content); i += 2 {
				key := ether.Content[i]
				if key.Value != item.ID {
					continue
				}
				// Change only the key, preserving comments, unknown fields and layout.
				lines := strings.SplitAfter(string(file.Data), "\n")
				start := key.Column - 1
				for _, line := range lines[:key.Line-1] {
					start += len(line)
				}
				colon := bytes.IndexByte(file.Data[start:], ':')
				if colon < 0 {
					return nil, fmt.Errorf("network: invalid definition key span")
				}
				file.Data = append(append(append([]byte(nil), file.Data[:start]...), desired...), file.Data[start+colon:]...)
				if err := parseFile(file); err != nil {
					return nil, err
				}
				changed[path] = file.Data
				break
			}
		}
	}
	return changed, nil
}

// Reject references rather than silently orphaning bridge/bond/VLAN membership
// or advanced Netplan extensions. Only the definition being changed is skipped.
func (s *Snapshot) referencesDefinition(id string) bool {
	var references func(*yaml.Node) bool
	references = func(n *yaml.Node) bool {
		if n.Kind == yaml.ScalarNode && n.Value == id {
			return true
		}
		for _, c := range n.Content {
			if references(c) {
				return true
			}
		}
		return false
	}
	for _, file := range s.Files {
		network := child(file.doc.Content[0], "network")
		for i := 0; i < len(network.Content); i += 2 {
			key, value := network.Content[i], network.Content[i+1]
			if key.Value != "ethernets" {
				if references(value) {
					return true
				}
				continue
			}
			for j := 0; j < len(value.Content); j += 2 {
				if value.Content[j].Value != id && references(value.Content[j+1]) {
					return true
				}
			}
		}
	}
	return false
}

func normalizeDefinitionMapping(file *File) error {
	network := child(file.doc.Content[0], "network")
	inline := false
	for _, node := range []*yaml.Node{file.doc.Content[0], network, child(network, "ethernets")} {
		inline = inline || node != nil && node.Style&yaml.FlowStyle != 0
	}
	if ether := child(network, "ethernets"); ether != nil {
		for i := 1; i < len(ether.Content); i += 2 {
			inline = inline || ether.Content[i].Style&yaml.FlowStyle != 0
		}
	}
	if !inline {
		return nil
	}
	doc := copyNode(file.doc)
	blockYAML(doc)
	var data bytes.Buffer
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if bytes.Contains(file.Data, []byte("\r\n")) {
		file.Data = bytes.ReplaceAll(data.Bytes(), []byte("\n"), []byte("\r\n"))
	} else {
		file.Data = data.Bytes()
	}
	return parseFile(file)
}
