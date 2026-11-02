package networkmanager

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

func blockYAML(node *yaml.Node) {
	node.Style &^= yaml.FlowStyle
	for _, child := range node.Content {
		blockYAML(child)
	}
}

// Inline YAML is valid Netplan. Normalize only affected inline source files in
// memory before applying field spans. Preserve comments and unknown properties;
// save still writes only the private candidate, and confirm commits the result.
func (s *Snapshot) normalizeEditedFlowSources(view View) (*Snapshot, map[string][]byte, error) {
	indices := map[int]bool{}
	for name, old := range s.View.Interfaces {
		next, exists := view.Interfaces[name]
		if !exists || old.ReadOnly != "" || equal(old, next) && equal(routesFor(s.View, name), routesFor(view, name)) {
			continue
		}
		for _, path := range old.Files {
			index := s.sourceIndex(path)
			if index < 0 {
				continue
			}
			root := s.Files[index].doc.Content[0]
			network := child(root, "network")
			ether := child(network, "ethernets")
			for _, node := range []*yaml.Node{root, network, ether} {
				if node != nil && node.Style&yaml.FlowStyle != 0 {
					indices[index] = true
				}
			}
			for _, fragment := range s.fragments[name] {
				if fragment.file == index && fragment.node.Style&yaml.FlowStyle != 0 {
					indices[index] = true
				}
			}
		}
	}
	result := map[string][]byte{}
	if len(indices) == 0 {
		return s, result, nil
	}
	working := *s
	working.Files = append([]File(nil), s.Files...)
	working.fragments = map[string][]fragment{}
	for name, fragments := range s.fragments {
		working.fragments[name] = append([]fragment(nil), fragments...)
	}
	for index := range indices {
		file := &working.Files[index]
		doc := copyNode(file.doc)
		blockYAML(doc)
		var data bytes.Buffer
		encoder := yaml.NewEncoder(&data)
		encoder.SetIndent(2)
		if err := encoder.Encode(doc); err != nil {
			return nil, nil, err
		}
		if err := encoder.Close(); err != nil {
			return nil, nil, err
		}
		file.Data = data.Bytes()
		if bytes.Contains(s.Files[index].Data, []byte("\r\n")) {
			file.Data = bytes.ReplaceAll(file.Data, []byte("\n"), []byte("\r\n"))
		}
		if err := parseFile(file); err != nil {
			return nil, nil, err
		}
		for name, fragments := range working.fragments {
			for position, fragment := range fragments {
				if fragment.file != index {
					continue
				}
				node := child(child(child(file.doc.Content[0], "network"), "ethernets"), fragment.id)
				if node == nil {
					return nil, nil, fmt.Errorf("network: could not normalize interface %s", name)
				}
				working.fragments[name][position].node = node
			}
		}
		result[file.Path] = file.Data
	}
	return &working, result, nil
}
