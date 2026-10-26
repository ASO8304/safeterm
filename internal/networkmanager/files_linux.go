//go:build linux

package networkmanager

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func checkParents(root, path string) error {
	expected := uint32(0)
	if root != "/" {
		expected = uint32(os.Geteuid())
	}
	resolved, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	for dir := resolved; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || st.Uid != expected || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("network: unsafe directory %s", dir)
		}
		if dir == filepath.Clean(root) || dir == "/" {
			break
		}
		if root != "/" && !strings.HasPrefix(dir, filepath.Clean(root)+string(filepath.Separator)) {
			return fmt.Errorf("network: path escapes test root")
		}
	}
	return nil
}
func readTrusted(root, relative string) (File, error) {
	path := filepath.Join(root, relative)
	if err := checkParents(root, path); err != nil {
		return File{}, err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return File{}, &os.PathError{Op: "open", Path: relative, Err: err}
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return File{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	expected := uint32(0)
	if root != "/" {
		expected = uint32(os.Geteuid())
	}
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || st.Uid != expected || st.Nlink != 1 || info.Size() > 1<<20 {
		return File{}, fmt.Errorf("network: %s must be a trusted single-link regular file <=1 MiB without group/world write", relative)
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return File{}, fmt.Errorf("network: cannot read bounded file %s", relative)
	}
	attrs, err := readAttrs(fd)
	if err != nil {
		return File{}, err
	}
	for name := range attrs {
		if name == "security.ima" || name == "security.evm" || name == "security.capability" {
			return File{}, fmt.Errorf("network: content signatures or executable capabilities cannot be preserved when editing %s", relative)
		}
	}
	return File{Path: filepath.ToSlash(relative), Data: data, Mode: uint32(info.Mode().Perm()), UID: st.Uid, GID: st.Gid, Attrs: attrs}, nil
}
func readAttrs(fd int) (map[string][]byte, error) {
	attrs := map[string][]byte{}
	n, err := unix.Flistxattr(fd, nil)
	if err == unix.ENOTSUP {
		return attrs, nil
	}
	if err != nil {
		return nil, err
	}
	if n > 65536 {
		return nil, fmt.Errorf("network: excessive extended attributes")
	}
	names := make([]byte, n)
	n, err = unix.Flistxattr(fd, names)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, name := range strings.Split(string(names[:n]), "\x00") {
		if name == "" {
			continue
		}
		size, err := unix.Fgetxattr(fd, name, nil)
		if err != nil {
			return nil, err
		}
		total += size
		if total > 65536 {
			return nil, fmt.Errorf("network: excessive extended attributes")
		}
		value := make([]byte, size)
		size, err = unix.Fgetxattr(fd, name, value)
		if err != nil {
			return nil, err
		}
		attrs[name] = value[:size]
	}
	return attrs, nil
}
func replaceFile(root string, old File, data []byte) error {
	current, err := readTrusted(root, old.Path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current.Data, old.Data) || current.Mode != old.Mode || current.UID != old.UID || current.GID != old.GID || !equal(current.Attrs, old.Attrs) {
		return fmt.Errorf("network: source changed: %s", old.Path)
	}
	path := filepath.Join(root, old.Path)
	temp, err := os.CreateTemp(filepath.Dir(path), ".safeterm-netplan-")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	defer temp.Close()
	if _, err = temp.Write(data); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		if err = temp.Chown(int(old.UID), int(old.GID)); err != nil {
			return err
		}
	}
	if err = temp.Chmod(os.FileMode(old.Mode)); err != nil {
		return err
	}
	for key, value := range old.Attrs {
		if err = unix.Fsetxattr(int(temp.Fd()), key, value, 0); err != nil {
			return err
		}
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	// Recheck after preparing the replacement. Uncooperative privileged editors
	// cannot be locked out; unexpected changes are refused whenever observed.
	latest, err := readTrusted(root, old.Path)
	if err != nil || !sameFile(latest, old) {
		return fmt.Errorf("network: source changed before replacement: %s", old.Path)
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func privateWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".network-state-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	if err = file.Chmod(0600); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func checkPrivateDir(root, path string) error {
	if err := checkParents(root, filepath.Join(path, "placeholder")); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("network: state directory must be private mode 0700")
	}
	return nil
}
func readPrivate(root, path string) ([]byte, error) {
	if err := checkPrivateDir(root, filepath.Dir(path)); err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	expected := uint32(0)
	if root != "/" {
		expected = uint32(os.Geteuid())
	}
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || st.Uid != expected || st.Nlink != 1 || info.Size() > 16<<20 {
		return nil, fmt.Errorf("network: unsafe state file")
	}
	return io.ReadAll(io.LimitReader(file, 16<<20))
}
func writeNew(root, path string, data []byte) error {
	if err := checkParents(root, path); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".safeterm-netplan-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	if err = file.Chmod(0600); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = unix.Renameat2(unix.AT_FDCWD, name, unix.AT_FDCWD, path, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ReadTrustedData reads a bounded root-managed file without following a final symlink.
func ReadTrustedData(root, relative string) ([]byte, error) {
	f, err := readTrusted(root, relative)
	return f.Data, err
}

// LockState must be acquired before recovery. A second daemon must never undo a live trial.
func LockState(root, state string) (*os.File, error) {
	if err := os.MkdirAll(state, 0700); err != nil {
		return nil, err
	}
	if err := checkPrivateDir(root, state); err != nil {
		return nil, err
	}
	fd, err := unix.Open(filepath.Join(state, "manager.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "network-manager-lock")
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		f.Close()
		return nil, fmt.Errorf("network: unsafe lock file")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("network: another manager is active: %w", err)
	}
	return f, nil
}

// LifecycleLock excludes package/catalog changes for the complete duration of a trial.
func LifecycleLock() (io.Closer, error) {
	path := "/run/lock/safeterm-lifecycle.lock"
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || st.Uid != 0 || st.Nlink != 1 {
		f.Close()
		return nil, fmt.Errorf("network: unsafe lifecycle lock")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("network: a SafeTerm lifecycle operation is active")
	}
	return f, nil
}
