package networkmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"safeterm/internal/contract"
)

type namingTransaction struct {
	Files   []File
	Changed map[string][]byte
}

func (s *Snapshot) bindInterfaceMAC(name, mac string) (map[string][]byte, error) {
	patches := map[int][]textPatch{}
	fragments := s.fragments[name]
	if len(fragments) == 0 {
		index := s.managedSourceIndex()
		if index < 0 {
			return nil, fmt.Errorf("network: interface has no writable Netplan source")
		}
		network := child(s.Files[index].doc.Content[0], "network")
		ethernet := child(network, "ethernets")
		item := s.View.Interfaces[name]
		node := interfaceNode(item, mac, routesFor(s.View, name))
		key, mapping := item.ID, ethernet
		if ethernet == nil || len(ethernet.Content) == 0 {
			wrapper := nodeFor(map[string]string{})
			setChild(wrapper, item.ID, node)
			node, key, mapping = wrapper, "ethernets", network
		}
		patch, err := fieldPatch(s.Files[index], mapping, key, node)
		if err != nil {
			return nil, err
		}
		patches[index] = append(patches[index], patch)
	} else {
		for index, fragment := range fragments {
			var match = nodeFor(map[string]string{"macaddress": mac})
			if index != len(fragments)-1 {
				match = nil
			}
			for _, key := range []string{"match", "set-name"} {
				value := match
				if key == "set-name" {
					value = nil
				}
				if value == nil && child(fragment.node, key) == nil {
					continue
				}
				patch, err := fieldPatch(s.Files[fragment.file], fragment.node, key, value)
				if err != nil {
					return nil, err
				}
				patches[fragment.file] = append(patches[fragment.file], patch)
			}
		}
	}
	return s.renderPatches(patches)
}

func (m *Manager) nameInterface(ctx context.Context, req *contract.ExecutionRequest, request Request, policy Policy, snapshot *Snapshot, hasCandidate bool) (Reply, error) {
	if !interfaceNamePattern.MatchString(request.NewName) || request.NewName == "lo" {
		return Reply{}, fmt.Errorf("network: new name must start with a letter, use letters/digits/_/-, and be at most 15 characters")
	}
	item, exists := snapshot.View.Interfaces[request.Interface]
	if !exists || item.ReadOnly != "" {
		return Reply{}, fmt.Errorf("network: renaming requires a present, editable interface allowed by policy")
	}
	if !policy.AllowsInterface(request.NewName) {
		return Reply{}, fmt.Errorf("network: allow the future name %q in network/policy.json or enable allow_all_interfaces before renaming", request.NewName)
	}
	if hasCandidate {
		return Reply{}, fmt.Errorf("network: save/apply/confirm or discard the existing network candidate before renaming")
	}
	mac, valid := normalizedMAC(snapshot.View.MACs[request.Interface])
	if !valid {
		return Reply{}, fmt.Errorf("network: renaming requires a valid Ethernet MAC address")
	}
	for name, value := range m.opts.Interfaces() {
		if name != request.Interface && (strings.EqualFold(value, mac) || name == request.NewName) {
			return Reply{}, fmt.Errorf("network: MAC address or requested name conflicts with another present interface")
		}
	}
	rules, err := readLinkRules(m.opts.Root)
	if err != nil {
		return Reply{}, err
	}
	for _, rule := range rules {
		if rule.name == request.NewName && !contains(rule.macs, mac) {
			return Reply{}, fmt.Errorf("network: requested name is reserved by another .link rule: %s", rule.file.Path)
		}
	}
	link, err := selectLinkRule(m.opts.Root, mac, rules)
	if err != nil {
		return Reply{}, err
	}
	changed, err := snapshot.bindInterfaceMAC(request.Interface, mac)
	if err != nil {
		return Reply{}, err
	}
	if err := m.acquireTrialLock(); err != nil {
		return Reply{}, err
	}
	defer m.releaseTrialLock()
	if err := m.opts.Backend.Validate(ctx, snapshot.Files, changed); err != nil {
		return Reply{}, err
	}
	currentPolicy, err := m.opts.Policy()
	if err != nil || policyHash(currentPolicy) != policyHash(policy) {
		return Reply{}, fmt.Errorf("network: administrator policy changed during naming validation; retry")
	}
	latest, err := m.snapshot(policy)
	if err != nil || latest.View.Version != snapshot.View.Version {
		return Reply{}, fmt.Errorf("network: sources or interfaces changed during naming validation; retry")
	}
	if err := prepareNamingDirectory(m.opts.Root); err != nil {
		return Reply{}, err
	}
	tx := namingTransaction{Changed: changed}
	for _, file := range snapshot.Files {
		if _, changed := changed[file.Path]; changed {
			tx.Files = append(tx.Files, file)
		}
	}
	// The .link is committed last. No udev trigger, netplan apply, service
	// restart, live rename or reboot is executed by this operation.
	tx.Files = append(tx.Files, link)
	tx.Changed[link.Path] = renameLink(link, mac, request.NewName)
	if err := m.report(ctx, req, "apply-authorized", snapshot.View.Version); err != nil {
		return Reply{}, err
	}
	if err := m.save("naming-pending.json", tx); err != nil {
		return Reply{}, err
	}
	for _, file := range tx.Files {
		if err := replaceFile(m.opts.Root, file, tx.Changed[file.Path]); err != nil {
			recovery := m.recoverNaming()
			return Reply{}, fmt.Errorf("network: persistent naming failed: %v; recovery: %v", err, recovery)
		}
	}
	if err := m.remove("naming-pending.json"); err != nil {
		return Reply{}, err
	}
	if err := m.report(ctx, req, "applied", "network-v1"); err != nil {
		return Reply{}, fmt.Errorf("network: name persisted, completion audit unavailable: %w", err)
	}
	return Reply{Text: fmt.Sprintf("Persistent name: %s -> %s (MAC %s).\nUpdated /%s and bound its Netplan definition to the MAC.\nTakes effect on next reboot/device activation; the live interface was not renamed.", request.Interface, request.NewName, mac, link.Path)}, nil
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
