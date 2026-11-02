package networkmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (tx *transaction) changedData(file File) []byte {
	if filepath.Dir(file.Path) == "etc/systemd/network" {
		return tx.LinkChanged[file.Path]
	}
	return tx.Changed[file.Path]
}

// A fully written naming commit may already have been consumed by udev during
// reboot. Finalize it rather than restoring an obsolete name and Netplan match.
func (m *Manager) recoverCandidateNames(tx *transaction) (bool, error) {
	if len(tx.LinkFiles) == 0 || (tx.Phase != "committing" && tx.Phase != "confirmed") {
		return false, nil
	}
	if err := validTransaction(tx); err != nil {
		return false, err
	}
	for _, file := range append(append([]File(nil), tx.Files...), tx.LinkFiles...) {
		current, err := readTrusted(m.opts.Root, file.Path)
		expected := file
		expected.Data, expected.Missing = tx.changedData(file), false
		if err != nil || !sameFile(current, expected) {
			return false, nil
		}
	}
	if err := m.removeOverrides(tx); err != nil {
		return false, err
	}
	if err := m.save("route-names.json", tx.NewNames); err != nil {
		return false, err
	}
	tx.Phase = "confirmed"
	if err := m.save("last.json", tx); err != nil {
		return false, err
	}
	if err := m.remove("candidate.json"); err != nil {
		return false, err
	}
	return true, m.remove("pending.json")
}

// Naming is part of the private network candidate. Planning never writes files
// or applies a name; the matching Netplan changes and link rules commit together.
func (m *Manager) planNames(snapshot *Snapshot, view View, policy Policy) ([]File, map[string][]byte, error) {
	rules, err := readLinkRules(m.opts.Root)
	if err != nil {
		return nil, nil, err
	}
	live := m.opts.Interfaces()
	names := []string{}
	for name, next := range view.Interfaces {
		if next.PersistentName != snapshot.View.Interfaces[name].PersistentName {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	files, changed := []File{}, map[string][]byte{}
	targets := map[string]string{}
	for _, name := range names {
		item := view.Interfaces[name]
		mac, valid := normalizedMAC(live[name])
		if !valid || item.ReadOnly != "" || !policy.AllowsInterface(name) || !policy.AllowsInterface(item.PersistentName) {
			return nil, nil, fmt.Errorf("network: persistent naming requires a present, editable interface and a future name allowed by network/policy.json")
		}
		if !interfaceNamePattern.MatchString(item.PersistentName) || item.PersistentName == "lo" {
			return nil, nil, fmt.Errorf("network: invalid persistent name %q", item.PersistentName)
		}
		if other, exists := targets[item.PersistentName]; exists && other != name {
			return nil, nil, fmt.Errorf("network: persistent name %q is requested by multiple interfaces", item.PersistentName)
		}
		targets[item.PersistentName] = name
		for other, address := range live {
			if other != name && (other == item.PersistentName || strings.EqualFold(address, mac)) {
				return nil, nil, fmt.Errorf("network: MAC address or requested name conflicts with another present interface")
			}
		}
		for _, rule := range rules {
			if rule.name == item.PersistentName && !contains(rule.macs, mac) {
				return nil, nil, fmt.Errorf("network: requested name is reserved by another .link rule: %s", rule.file.Path)
			}
		}
		file, err := selectLinkRule(m.opts.Root, mac, rules)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, file)
		changed[file.Path] = renameLink(file, mac, item.PersistentName)
		// Reserve newly selected filenames for other interfaces in this draft.
		if file.Missing {
			rules = append(rules, linkRule{file: file, macs: []string{mac}, name: item.PersistentName, simple: true})
		}
	}
	return files, changed, nil
}

func validLinkTransaction(files []File, changed map[string][]byte) error {
	if len(files) > maxFiles || len(files) != len(changed) {
		return fmt.Errorf("network: invalid link rule transaction")
	}
	seen := map[string]bool{}
	for _, file := range files {
		base := filepath.Base(file.Path)
		if filepath.Dir(file.Path) != "etc/systemd/network" || !namePattern.MatchString(base) || !strings.HasSuffix(base, ".link") || seen[file.Path] || len(changed[file.Path]) == 0 {
			return fmt.Errorf("network: invalid link rule recovery path")
		}
		seen[file.Path] = true
		if file.Missing && (file.Mode != 0600 || len(file.Attrs) != 0) {
			return fmt.Errorf("network: invalid new link rule recovery file")
		}
	}
	return nil
}

func (m *Manager) verifyLinkFiles(files []File) error {
	for _, file := range files {
		for _, directory := range []string{"etc", "run", "usr/lib", "usr/local/lib"} {
			path := filepath.Join(m.opts.Root, directory, "systemd/network", filepath.Base(file.Path)+".d")
			if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
				return fmt.Errorf("network: link rule gained drop-ins; discard and reload the candidate")
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if file.Missing && directory != "etc" {
				path = strings.TrimSuffix(path, ".d")
				if _, err := os.Lstat(path); err == nil {
					return fmt.Errorf("network: persistent naming filename became occupied: %s", path)
				} else if !os.IsNotExist(err) {
					return err
				}
			}
		}
		current, err := readTrusted(m.opts.Root, file.Path)
		if file.Missing && os.IsNotExist(err) {
			continue
		}
		if err != nil || !sameFile(current, file) {
			return fmt.Errorf("network: persistent naming source changed: %s", file.Path)
		}
	}
	return nil
}
