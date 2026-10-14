//go:build linux

package identity

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// CheckRootManaged verifies the deployed credential boundary before startup.
// Only root can replace this file or its parent directories; the identity group
// can read it. The shared Console UID must never belong to that group.
func CheckRootManaged(path string, group uint32) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0640 || stat.Uid != 0 || stat.Gid != group || stat.Nlink != 1 {
		return fmt.Errorf("identity: credentials must be root:identity-group mode 0640 without links")
	}
	if n, err := unix.Getxattr(path, "system.posix_acl_access", nil); n > 0 || (err != nil && err != unix.ENODATA && err != unix.ENOTSUP) {
		return fmt.Errorf("identity: credential files must not have extended access ACLs")
	}
	entry, err := user.Lookup("safeterm")
	if err != nil {
		return err
	}
	if entry.Uid == "0" {
		return fmt.Errorf("identity: Console entry must not be root")
	}
	groups, err := entry.GroupIds()
	if err != nil {
		return err
	}
	for _, id := range groups {
		if id == strconv.FormatUint(uint64(group), 10) {
			return fmt.Errorf("identity: shared Console account must not belong to the credential group")
		}
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("identity: credential directories must be root-owned without group or world write access")
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return nil
}
