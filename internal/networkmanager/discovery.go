package networkmanager

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type File struct {
	Path     string
	Data     []byte
	Mode     uint32
	UID, GID uint32
	Attrs    map[string][]byte
	doc      *yaml.Node
}
type fragment struct {
	file int
	id   string
	node *yaml.Node
}
type Snapshot struct {
	View       View
	Files      []File
	fragments  map[string][]fragment
	routeNodes map[string]*yaml.Node
}

func child(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}
func list(n *yaml.Node) ([]string, error) {
	if n == nil {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected a list on line %d", n.Line)
	}
	out := []string{}
	for _, v := range n.Content {
		if v.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("advanced address mapping on line %d is read-only", v.Line)
		}
		out = append(out, v.Value)
	}
	return out, nil
}
func checkTree(n *yaml.Node, depth int, count *int) error {
	*count++
	if depth > 32 || *count > 30000 {
		return fmt.Errorf("excessive YAML depth/nodes")
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return fmt.Errorf("YAML anchors/aliases are unsupported (line %d)", n.Line)
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" || seen[k.Value] {
				return fmt.Errorf("invalid or duplicate YAML key on line %d", k.Line)
			}
			seen[k.Value] = true
		}
	}
	for _, c := range n.Content {
		if err := checkTree(c, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}
func parseFile(f *File) error {
	decoder := yaml.NewDecoder(bytes.NewReader(f.Data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return fmt.Errorf("network: %s: %w", f.Path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("network: %s: exactly one YAML document required", f.Path)
	}
	count := 0
	if err := checkTree(&doc, 0, &count); err != nil {
		return fmt.Errorf("network: %s: %w", f.Path, err)
	}
	if len(doc.Content) != 1 || scalar(child(child(doc.Content[0], "network"), "version")) != "2" {
		return fmt.Errorf("network: %s: network.version must be 2", f.Path)
	}
	f.doc = &doc
	return nil
}

// Discover implements Netplan's filename shadowing and lexicographic merge.
// All directories contribute to the version, including shadowed files.
func Discover(root string, p Policy, names map[string]string) (*Snapshot, error) {
	selected := map[string]int{}
	files := []File{}
	total := 0
	for _, directory := range []string{"lib/netplan", "etc/netplan", "run/netplan"} {
		path := filepath.Join(root, directory)
		entries, err := os.ReadDir(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			if len(files) >= maxFiles {
				return nil, fmt.Errorf("network: too many Netplan files")
			}
			f, err := readTrusted(root, filepath.Join(directory, entry.Name()))
			if err != nil {
				return nil, err
			}
			total += len(f.Data)
			if total > maxBytes {
				return nil, fmt.Errorf("network: Netplan input exceeds 4 MiB")
			}
			if err := parseFile(&f); err != nil {
				return nil, err
			}
			selected[entry.Name()] = len(files)
			files = append(files, f)
		}
	}
	s := &Snapshot{Files: files, View: View{Interfaces: map[string]Interface{}, Routes: map[string]Route{}}, fragments: map[string][]fragment{}, routeNodes: map[string]*yaml.Node{}}
	ordered := []string{}
	for name := range selected {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		index := selected[name]
		f := files[index]
		network := child(f.doc.Content[0], "network")
		renderer := scalar(child(network, "renderer"))
		ether := child(network, "ethernets")
		if ether == nil {
			continue
		}
		if ether.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("network: %s: ethernets must be a mapping", f.Path)
		}
		for i := 0; i < len(ether.Content); i += 2 {
			id, n := ether.Content[i].Value, ether.Content[i+1]
			if !namePattern.MatchString(id) || n.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("network: invalid Ethernet definition in %s", f.Path)
			}
			item := s.View.Interfaces[id]
			item.ID = id
			item.Name = id
			item.Files = append(item.Files, f.Path)
			reason := ""
			if renderer != "" && renderer != "networkd" {
				reason = fmt.Sprintf("Interface controlled by unsupported Netplan renderer (%s).", renderer)
			}
			if r := scalar(child(n, "renderer")); r != "" && r != "networkd" {
				reason = fmt.Sprintf("Interface controlled by unsupported Netplan renderer (%s).", r)
			}
			if !strings.HasPrefix(f.Path, "etc/netplan/") {
				reason = "definition outside /etc/netplan is read-only"
			}
			if len(p.Files) > 0 && !contains(p.Files, filepath.Base(f.Path)) {
				reason = "source filename is outside administrator policy"
			}
			lower := strings.ToLower(string(f.Data))
			if !p.AllowGeneratedFiles && (strings.Contains(lower, "cloud-init") || strings.Contains(lower, "cloud_init")) {
				reason = "cloud-init generated file requires explicit administrator ownership acknowledgement"
			}
			if reason != "" {
				item.ReadOnly = reason
			}
			if node := child(n, "dhcp4"); node != nil {
				switch strings.ToLower(node.Value) {
				case "true", "yes", "on":
					item.DHCP4 = true
				case "false", "no", "off":
					item.DHCP4 = false
				default:
					return nil, fmt.Errorf("network: %s: invalid dhcp4", f.Path)
				}
			}
			for _, field := range []string{"addresses", "dns", "search-domains"} {
				var node *yaml.Node
				switch field {
				case "addresses":
					node = child(n, "addresses")
				case "dns":
					node = child(child(n, "nameservers"), "addresses")
				case "search-domains":
					node = child(child(n, "nameservers"), "search")
				}
				values, e := list(node)
				if e != nil {
					item.ReadOnly = e.Error()
					continue
				}
				switch field {
				case "addresses":
					item.Addresses = append(item.Addresses, values...)
				case "dns":
					item.DNS = append(item.DNS, values...)
				case "search-domains":
					item.SearchDomains = append(item.SearchDomains, values...)
				}
			}
			if node := child(n, "mtu"); node != nil {
				v, e := strconv.Atoi(node.Value)
				if e != nil {
					return nil, fmt.Errorf("network: %s: invalid mtu", f.Path)
				}
				item.MTU = &v
			}
			// Legacy gateway fields are imported as routes but remain read-only until
			// an administrator migrates them; do not create conflicting default routes.
			for _, legacy := range []string{"gateway4", "gateway6"} {
				if child(n, legacy) != nil {
					item.ReadOnly = "legacy gateway4/gateway6 must be migrated to routes by the administrator"
				}
			}
			routes := child(n, "routes")
			if routes != nil {
				if routes.Kind != yaml.SequenceNode {
					return nil, fmt.Errorf("network: routes must be a list")
				}
				for _, node := range routes.Content {
					raw, _ := yaml.Marshal(node)
					sum := sha256.Sum256(append([]byte(id+":"), raw...))
					routeName := "route-" + hex.EncodeToString(sum[:6])
					if _, exists := s.View.Routes[routeName]; exists {
						return nil, fmt.Errorf("network: duplicate route in Netplan files")
					}
					r := Route{Name: routeName, Interface: id, Destination: scalar(child(node, "to")), Gateway: scalar(child(node, "via"))}
					if r.Destination == "default" {
						r.Destination = "0.0.0.0/0"
						if strings.Contains(r.Gateway, ":") {
							r.Destination = "::/0"
						}
					}
					if m := child(node, "metric"); m != nil {
						v, e := strconv.ParseUint(m.Value, 10, 32)
						if e != nil {
							return nil, fmt.Errorf("network: invalid route metric")
						}
						vv := uint32(v)
						r.Metric = &vv
					}
					if node.Kind != yaml.MappingNode {
						r.ReadOnly = "advanced route is read-only"
					} else {
						for j := 0; j < len(node.Content); j += 2 {
							if !contains([]string{"to", "via", "metric"}, node.Content[j].Value) {
								r.ReadOnly = "advanced route options are preserved read-only"
							}
						}
					}
					s.View.Routes[routeName] = r
					s.routeNodes[routeName] = node
				}
			}
			s.View.Interfaces[id] = item
			s.fragments[id] = append(s.fragments[id], fragment{file: index, id: id, node: n})
		}
	}

	// Resolve simple physical name/MAC matches without exposing virtual or wildcard sets.
	resolved := map[string]string{}
	for id, item := range s.View.Interfaces {
		actual := id
		var match *yaml.Node
		rename := ""
		for _, fragment := range s.fragments[id] {
			if node := child(fragment.node, "match"); node != nil {
				match = node
			}
			if node := child(fragment.node, "set-name"); node != nil {
				rename = node.Value
			}
		}
		if match != nil || rename != "" {
			candidates := []string{}
			for name, mac := range names {
				ok := true
				if rename != "" && name != rename {
					ok = false
				}
				if match != nil {
					if match.Kind != yaml.MappingNode {
						ok = false
					} else {
						for j := 0; j < len(match.Content); j += 2 {
							key, value := match.Content[j].Value, match.Content[j+1].Value
							switch key {
							case "macaddress":
								ok = ok && strings.EqualFold(mac, value)
							case "name":
								ok = ok && (name == value || rename != "" && name == rename)
							default:
								ok = false
							}
						}
					}
				}
				if ok {
					candidates = append(candidates, name)
				}
			}
			if len(candidates) != 1 {
				item.ReadOnly = "interface match must resolve to exactly one present physical interface"
			} else {
				actual = candidates[0]
			}
		}
		item.Name = actual
		if !contains(p.Interfaces, actual) {
			item.ReadOnly = fmt.Sprintf("interface is outside administrator policy; a Linux administrator must add %q to interfaces in %s, then reopen config network interface", actual, PolicyPath)
		}
		if _, exists := names[actual]; !exists {
			item.ReadOnly = "physical interface is not present"
		}
		if previous, exists := resolved[actual]; exists && previous != id {
			return nil, fmt.Errorf("network: multiple Netplan IDs resolve to interface %s", actual)
		}
		resolved[actual] = id
		s.View.Interfaces[id] = item
	}

	mappedInterfaces := map[string]Interface{}
	mappedFragments := map[string][]fragment{}
	for actual, id := range resolved {
		mappedInterfaces[actual] = s.View.Interfaces[id]
		mappedFragments[actual] = s.fragments[id]
	}
	for name, r := range s.View.Routes {
		for actual, id := range resolved {
			if r.Interface == id {
				r.Interface = actual
				break
			}
		}
		s.View.Routes[name] = r
	}
	s.View.Interfaces = mappedInterfaces
	s.fragments = mappedFragments
	hash := sha256.New()
	for _, f := range files {
		data, _ := json.Marshal(struct {
			Path     string
			Data     []byte
			Mode     uint32
			UID, GID uint32
			Attrs    map[string][]byte
		}{f.Path, f.Data, f.Mode, f.UID, f.GID, f.Attrs})
		hash.Write(data)
	}
	policy, _ := json.Marshal(p)
	hash.Write(policy)
	s.View.Version = hex.EncodeToString(hash.Sum(nil))
	return s, nil
}
func contains(items []string, s string) bool {
	for _, v := range items {
		if s == v {
			return true
		}
	}
	return false
}
func equal(a, b any) bool { return reflect.DeepEqual(a, b) }

// Render edits only selected field spans. Unrelated bytes remain identical.
func (s *Snapshot) Render(v View) (map[string][]byte, error) {
	if err := ValidateView(v); err != nil {
		return nil, err
	}
	patches := map[int][]textPatch{}
	for name, old := range s.View.Interfaces {
		next, exists := v.Interfaces[name]
		if !exists {
			return nil, fmt.Errorf("network: physical interfaces cannot be deleted")
		}
		oldRoutes, nextRoutes := routesFor(s.View, name), routesFor(v, name)
		if equal(old, next) && equal(oldRoutes, nextRoutes) {
			continue
		}
		if old.ReadOnly != "" {
			return nil, fmt.Errorf("network: %s: %s", name, old.ReadOnly)
		}
		fragments := s.fragments[name]
		if len(fragments) == 0 {
			return nil, fmt.Errorf("network: missing interface source")
		}
		values := map[string]*yaml.Node{}
		if old.DHCP4 != next.DHCP4 {
			values["dhcp4"] = nodeFor(next.DHCP4)
		}
		if !equal(old.Addresses, next.Addresses) {
			values["addresses"] = nodeFor(nonNil(next.Addresses))
		}
		if !equal(old.MTU, next.MTU) {
			if next.MTU == nil {
				values["mtu"] = nil
			} else {
				values["mtu"] = nodeFor(*next.MTU)
			}
		}
		if !equal(old.DNS, next.DNS) || !equal(old.SearchDomains, next.SearchDomains) {
			// Merge nameserver mappings so unknown properties remain intact.
			merged := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			for _, f := range fragments {
				if n := child(f.node, "nameservers"); n != nil {
					for i := 0; i < len(n.Content); i += 2 {
						setChild(merged, n.Content[i].Value, copyNode(n.Content[i+1]))
					}
				}
			}
			setChild(merged, "addresses", nodeFor(nonNil(next.DNS)))
			setChild(merged, "search", nodeFor(nonNil(next.SearchDomains)))
			values["nameservers"] = merged
		}
		if !equal(oldRoutes, nextRoutes) {
			seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for _, r := range nextRoutes {
				var n *yaml.Node
				if original, ok := s.routeNodes[r.Name]; ok {
					n = copyNode(original)
				} else {
					n = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				}
				if r.ReadOnly != "" {
					seq.Content = append(seq.Content, n)
					continue
				}
				setChild(n, "to", nodeFor(r.Destination))
				if r.Gateway != "" {
					setChild(n, "via", nodeFor(r.Gateway))
				} else {
					setChild(n, "via", nil)
				}
				if r.Metric != nil {
					setChild(n, "metric", nodeFor(*r.Metric))
				} else {
					setChild(n, "metric", nil)
				}
				seq.Content = append(seq.Content, n)
			}
			values["routes"] = seq
		}
		keys := []string{}
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			last := fragments[len(fragments)-1]
			for _, f := range fragments {
				if child(f.node, key) != nil {
					last = f
				}
			}
			for _, f := range fragments {
				if child(f.node, key) == nil && f.file != last.file {
					continue
				}
				value := values[key]
				if f.file != last.file {
					value = nil
				}
				patch, err := fieldPatch(s.Files[f.file], f.node, key, value)
				if err != nil {
					return nil, err
				}
				patches[f.file] = append(patches[f.file], patch)
			}
		}
	}
	result := map[string][]byte{}
	for index, list := range patches {
		sort.SliceStable(list, func(i, j int) bool { return list[i].start > list[j].start })
		data := append([]byte(nil), s.Files[index].Data...)
		previous := len(data) + 1
		for _, p := range list {
			if p.end > previous && p.start != p.end {
				return nil, fmt.Errorf("network: overlapping YAML edits")
			}
			data = append(append(append([]byte(nil), data[:p.start]...), p.data...), data[p.end:]...)
			previous = p.start
		}
		f := s.Files[index]
		f.Data = data
		if err := parseFile(&f); err != nil {
			return nil, err
		}
		result[f.Path] = data
	}
	return result, nil
}
func routesFor(v View, name string) []Route {
	out := []Route{}
	for _, r := range v.Routes {
		if r.Interface == name {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
func nodeFor(value any) *yaml.Node { n := &yaml.Node{}; _ = n.Encode(value); return n }
func copyNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	out := *n
	out.Content = nil
	for _, c := range n.Content {
		out.Content = append(out.Content, copyNode(c))
	}
	return &out
}
func setChild(n *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			if value == nil {
				n.Content = append(n.Content[:i], n.Content[i+2:]...)
			} else {
				n.Content[i+1] = value
			}
			return
		}
	}
	if value != nil {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	}
}

type textPatch struct {
	start, end int
	data       []byte
}

func fieldPatch(f File, mapping *yaml.Node, key string, value *yaml.Node) (textPatch, error) {
	if mapping.Style&yaml.FlowStyle != 0 {
		return textPatch{}, fmt.Errorf("network: flow-style Ethernet mappings are read-only")
	}
	lines := strings.SplitAfter(string(f.Data), "\n")
	offsets := make([]int, len(lines)+1)
	for i, line := range lines {
		offsets[i+1] = offsets[i] + len(line)
	}
	keyNode := (*yaml.Node)(nil)
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			keyNode = mapping.Content[i]
			break
		}
	}
	indent := mapping.Column - 1
	if len(mapping.Content) > 0 {
		indent = mapping.Content[0].Column - 1
	}
	start, end := 0, 0
	comments := []string{}
	if keyNode != nil {
		if keyNode.Column-1 != indent {
			return textPatch{}, fmt.Errorf("network: inconsistent YAML indentation")
		}
		start = keyNode.Line - 1
		end = start + 1
		for end < len(lines) {
			trim := strings.TrimSpace(lines[end])
			leading := len(lines[end]) - len(strings.TrimLeft(lines[end], " "))
			if trim != "" && !strings.HasPrefix(trim, "#") && leading <= indent {
				break
			}
			end++
		}
		for _, line := range lines[start:end] {
			if c := commentPart(line); c != "" {
				comments = append(comments, strings.Repeat(" ", indent)+c+"\n")
			}
		}
	} else {
		start = mapping.Line - 1
		if len(mapping.Content) > 0 {
			start = mapping.Content[len(mapping.Content)-2].Line - 1
		}
		end = start + 1
		for end < len(lines) {
			trim := strings.TrimSpace(lines[end])
			leading := len(lines[end]) - len(strings.TrimLeft(lines[end], " "))
			if trim != "" && !strings.HasPrefix(trim, "#") && leading <= indent {
				break
			}
			end++
		}
		start = end
	}
	newline := "\n"
	if bytes.Contains(f.Data, []byte("\r\n")) {
		newline = "\r\n"
	}
	out := strings.Join(comments, "")
	if value != nil {
		v := copyNode(value)
		stripComments(v)
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v}}
		var buffer bytes.Buffer
		encoder := yaml.NewEncoder(&buffer)
		encoder.SetIndent(2)
		if err := encoder.Encode(entry); err != nil {
			return textPatch{}, err
		}
		_ = encoder.Close()
		for _, line := range strings.Split(strings.TrimSuffix(buffer.String(), "\n"), "\n") {
			out += strings.Repeat(" ", indent) + line + "\n"
		}
	}
	if newline != "\n" {
		out = strings.ReplaceAll(out, "\n", newline)
	}
	if start > 0 && !strings.HasSuffix(lines[start-1], "\n") && out != "" {
		out = newline + out
	}
	return textPatch{start: offsets[start], end: offsets[end], data: []byte(out)}, nil
}
func stripComments(n *yaml.Node) {
	n.HeadComment = ""
	n.LineComment = ""
	n.FootComment = ""
	for _, c := range n.Content {
		stripComments(c)
	}
}
func commentPart(line string) string {
	quote := byte(0)
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == quote && !escaped {
				quote = 0
			}
			if quote == '"' && c == '\\' && !escaped {
				escaped = true
			} else {
				escaped = false
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		}
		if c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return strings.TrimRight(line[i:], "\r\n")
		}
	}
	return ""
}
