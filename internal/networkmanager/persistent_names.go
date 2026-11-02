package networkmanager

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type namingTransaction struct {
	Files   []File
	Changed map[string][]byte
}

func prepareNamingDirectory(root string) error {
	for _, relative := range []string{"etc/systemd", "etc/systemd/network"} {
		path := filepath.Join(root, relative)
		if err := checkParents(root, path); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if err = os.Mkdir(path, 0755); err != nil {
				return err
			}
			info, err = os.Lstat(path)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("network: unsafe persistent naming directory: %s", path)
		}
		if err := checkParents(root, filepath.Join(path, "placeholder")); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) recoverNaming() error {
	var tx namingTransaction
	if err := m.load("naming-pending.json", &tx); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if len(tx.Files) == 0 || len(tx.Files) > maxFiles || len(tx.Changed) != len(tx.Files) {
		return fmt.Errorf("network: invalid persistent naming recovery")
	}
	seen := map[string]bool{}
	for _, file := range tx.Files {
		base, directory := filepath.Base(file.Path), filepath.Dir(file.Path)
		allowed := directory == "etc/netplan" && strings.HasSuffix(base, ".yaml") || directory == "etc/systemd/network" && strings.HasSuffix(base, ".link")
		if !allowed || !namePattern.MatchString(base) || seen[file.Path] || len(tx.Changed[file.Path]) == 0 {
			return fmt.Errorf("network: invalid naming recovery path")
		}
		seen[file.Path] = true
	}
	// The link file is written last. If all replacements reached disk, finish
	// the commit rather than restoring a name rule that udev may already have
	// used during a reboot after a power failure.
	complete := true
	for _, old := range tx.Files {
		current, err := readTrusted(m.opts.Root, old.Path)
		expected := old
		expected.Data, expected.Missing = tx.Changed[old.Path], false
		if err == nil && sameFile(current, expected) {
			continue
		}
		complete = false
		if old.Missing && os.IsNotExist(err) || err == nil && sameFile(current, old) {
			continue
		}
		return fmt.Errorf("network: external edits prevent persistent naming recovery: %s", old.Path)
	}
	if complete {
		return m.remove("naming-pending.json")
	}
	for index := len(tx.Files) - 1; index >= 0; index-- {
		old := tx.Files[index]
		current, err := readTrusted(m.opts.Root, old.Path)
		if old.Missing && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if sameFile(old, current) {
			continue
		}
		expected := old
		expected.Data, expected.Missing = tx.Changed[old.Path], false
		if !sameFile(current, expected) {
			return fmt.Errorf("network: external edits prevent persistent naming recovery: %s", old.Path)
		}
		if old.Missing {
			err = removeTrusted(m.opts.Root, current)
		} else {
			err = replaceFile(m.opts.Root, current, old.Data)
		}
		if err != nil {
			return err
		}
	}
	return m.remove("naming-pending.json")
}

func addPersistentNames(snapshot *Snapshot, rules []linkRule) {
	snapshot.View.Names = map[string]string{}
	for name, mac := range snapshot.View.MACs {
		matches := 0
		for _, rule := range rules {
			if rule.simple && len(rule.macs) == 1 && strings.EqualFold(rule.macs[0], mac) {
				matches++
				snapshot.View.Names[name] = rule.name
			}
		}
		if matches != 1 {
			delete(snapshot.View.Names, name)
		}
	}
	for name, item := range snapshot.View.Interfaces {
		item.PersistentName = snapshot.View.Names[name]
		snapshot.View.Interfaces[name] = item
	}
	if len(rules) > 0 {
		hash := sha256.New()
		hash.Write([]byte(snapshot.View.Version))
		hash.Write([]byte(namingRulesDigest(rules)))
		snapshot.View.Version = hex.EncodeToString(hash.Sum(nil))
	}
}

func namingRulesDigest(rules []linkRule) string {
	ordered := append([]linkRule(nil), rules...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].file.Path < ordered[j].file.Path })
	hash := sha256.New()
	for _, rule := range ordered {
		hash.Write([]byte(fileDigest(rule.file)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
