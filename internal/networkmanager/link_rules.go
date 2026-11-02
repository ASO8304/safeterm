package networkmanager

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var interfaceNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,14}$`)

type linkRule struct {
	file   File
	macs   []string
	name   string
	simple bool
}

func readLinkRules(root string) ([]linkRule, error) {
	entries, err := os.ReadDir(filepath.Join(root, "etc/systemd/network"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rules := []linkRule{}
	total := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".link") {
			continue
		}
		if len(rules) >= maxFiles || !namePattern.MatchString(entry.Name()) {
			return nil, fmt.Errorf("network: too many or unsupported link rule filenames")
		}
		file, err := readTrusted(root, "etc/systemd/network/"+entry.Name())
		if err != nil {
			return nil, err
		}
		total += len(file.Data)
		if total > maxBytes {
			return nil, fmt.Errorf("network: link rules exceed 4 MiB")
		}
		rule := linkRule{file: file, simple: true}
		section := ""
		seen := map[string]bool{}
		for _, raw := range strings.Split(string(file.Data), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = line[1 : len(line)-1]
				if seen["["+section+"]"] {
					rule.simple = false
				}
				seen["["+section+"]"] = true
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !ok || strings.HasSuffix(value, "\\") {
				rule.simple = false
				continue
			}
			if seen[section+"/"+key] {
				rule.simple = false
			}
			seen[section+"/"+key] = true
			if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
			if section == "Match" {
				if key != "MACAddress" {
					rule.simple = false
					continue
				}
				for _, value := range strings.Fields(value) {
					mac, valid := normalizedMAC(value)
					if valid {
						rule.macs = append(rule.macs, mac)
					} else {
						rule.simple = false
					}
				}
			}
			if section == "Link" && key == "Name" {
				rule.name = value
				if !interfaceNamePattern.MatchString(value) {
					rule.simple = false
				}
			}
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func selectLinkRule(root, mac string, rules []linkRule) (File, error) {
	matches := []linkRule{}
	for _, rule := range rules {
		if contains(rule.macs, mac) {
			matches = append(matches, rule)
		}
	}
	if len(matches) > 1 {
		return File{}, fmt.Errorf("network: multiple .link files match MAC %s; a Linux administrator must resolve the ambiguity", mac)
	}
	if len(matches) == 1 {
		rule := matches[0]
		if !rule.simple || len(rule.macs) != 1 {
			return File{}, fmt.Errorf("network: %s has shared or advanced matches; split it into a single-MAC rule before renaming", rule.file.Path)
		}
		// A systemd drop-in could override Name or Match. Do not silently edit
		// the main file while a second source controls the effective setting.
		for _, directory := range []string{"etc", "run", "usr/lib", "usr/local/lib"} {
			path := filepath.Join(root, directory, "systemd/network", filepath.Base(rule.file.Path)+".d")
			if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
				return File{}, fmt.Errorf("network: link rule has drop-ins; a Linux administrator must resolve them before renaming")
			} else if err != nil && !os.IsNotExist(err) {
				return File{}, err
			}
		}
		return rule.file, nil
	}
	path := "etc/systemd/network/10-" + strings.ReplaceAll(mac, ":", "") + "-persistent-net.link"
	if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
		return File{}, fmt.Errorf("network: reserved link rule path already exists: %s", path)
	}
	return File{Path: path, Mode: 0600, UID: uint32(os.Geteuid()), GID: uint32(os.Getegid()), Attrs: map[string][]byte{}, Missing: true}, nil
}

func renameLink(file File, mac, name string) []byte {
	if file.Missing {
		return []byte("# Persistent interface naming managed by SafeTerm.\n[Match]\nMACAddress=" + mac + "\n\n[Link]\nNamePolicy=\nName=" + name + "\n")
	}
	newline := "\n"
	if bytes.Contains(file.Data, []byte("\r\n")) {
		newline = "\r\n"
	}
	lines := strings.SplitAfter(string(file.Data), "\n")
	section, linkEnd := "", len(lines)
	hasLink, hasName, hasPolicy := false, false, false
	for index, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if section == "Link" {
				linkEnd = index
			}
			section = line[1 : len(line)-1]
			hasLink = hasLink || section == "Link"
			continue
		}
		if section != "Link" {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		indent := raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]
		switch strings.TrimSpace(key) {
		case "Name":
			lines[index], hasName = indent+"Name="+name+newline, true
		case "NamePolicy":
			lines[index], hasPolicy = indent+"NamePolicy="+newline, true
		}
	}
	addition := ""
	if !hasLink {
		addition += newline + "[Link]" + newline
	}
	if !hasPolicy {
		addition += "NamePolicy=" + newline
	}
	if !hasName {
		addition += "Name=" + name + newline
	}
	before := strings.Join(lines[:linkEnd], "")
	if before != "" && !strings.HasSuffix(before, "\n") && addition != "" {
		before += newline
	}
	return []byte(before + addition + strings.Join(lines[linkEnd:], ""))
}
