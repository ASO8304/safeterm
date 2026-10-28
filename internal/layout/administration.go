package layout

import (
	"fmt"
	"os"
	"path/filepath"
)

// FlatAdministration lists only human-owned inputs from the v0.27 layout.
// Compiled catalogs are deliberately excluded: RejectLegacy still refuses them.
var FlatAdministration = [][2]string{
	{"ssh", "access/ssh"}, {"console", "access/console"},
	{"roles.d", "access/roles.d"}, {"policies.d", "access/policies.d"},
	{"operations.d", "commands/operations.d"}, {"executables.d", "commands/executables.d"}, {"profiles.d", "commands/profiles.d"},
	{"network.json", "network/policy.json"},
	{"NETWORK.md", "docs/network.md"}, {"NETWORK.txt", "docs/network.txt"},
	{"NETWORK-DIAGNOSTICS.md", "docs/network-diagnostics.md"}, {"NETWORK-DIAGNOSTICS.txt", "docs/network-diagnostics.txt"},
	{"README.txt", "docs/administration.txt"},
}

func CheckAdministration(admin string) error {
	if err := RejectLegacy(admin); err != nil {
		return err
	}
	for _, move := range FlatAdministration {
		if _, err := os.Lstat(filepath.Join(admin, move[0])); err == nil {
			return fmt.Errorf("administrator layout needs an upgrade: %s moves to %s; run sudo termctl update with the new release package", move[0], move[1])
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func administrationMoves(admin string) ([][2]string, error) {
	if err := RejectLegacy(admin); err != nil {
		return nil, err
	}
	for _, dir := range []string{"", "access", "commands", "network", "docs"} {
		path := filepath.Join(admin, dir)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("unsafe administrator directory %s", path)
		}
	}
	var moves [][2]string
	for _, move := range FlatAdministration {
		src, dst := filepath.Join(admin, move[0]), filepath.Join(admin, move[1])
		info, err := os.Lstat(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("unsafe administrator input %s", src)
		}
		if _, err := os.Lstat(dst); err == nil {
			return nil, fmt.Errorf("administrator layout conflict: both %s and %s exist; resolve before updating", src, dst)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		moves = append(moves, [2]string{src, dst})
	}
	return moves, nil
}

// CheckAdministrationMigration is read-only and runs during installer preflight.
func CheckAdministrationMigration(admin string) error {
	_, err := administrationMoves(admin)
	return err
}

// MigrateAdministration runs only inside installation, after the caller has
// snapshotted /etc/safeterm and stopped consumers. Rename preserves ownership,
// modes, ACLs and file contents. Repeating after interruption is safe. Validate
// all collisions and parent directories before moving anything; never merge.
func MigrateAdministration(admin string) error {
	moves, err := administrationMoves(admin)
	if err != nil {
		return err
	}
	for _, move := range moves {
		if err := os.MkdirAll(filepath.Dir(move[1]), 0755); err != nil {
			return err
		}
		if err := os.Rename(move[0], move[1]); err != nil {
			return err
		}
	}
	return nil
}
