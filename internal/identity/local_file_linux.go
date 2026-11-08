//go:build linux

package identity

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func readLocalUsers(path string) ([]byte, error) {
	// Installed account-to-role mappings are an authorization boundary.
	if path == LocalUsersPath {
		for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
			info, err := os.Lstat(dir)
			if err != nil {
				return nil, err
			}
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok || !info.IsDir() || st.Uid != 0 || info.Mode().Perm()&0022 != 0 {
				return nil, fmt.Errorf("identity: unsafe Linux mapping directory")
			}
			if dir == "/" {
				break
			}
		}
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	expected := uint32(os.Geteuid())
	if path == LocalUsersPath {
		expected = 0
	}
	if !ok || !info.Mode().IsRegular() || st.Nlink != 1 || st.Uid != expected || info.Mode().Perm()&0022 != 0 || info.Size() > 1<<20 {
		return nil, fmt.Errorf("identity: unsafe Linux mapping file")
	}
	// An access ACL could grant writes that the mode bits alone do not reveal.
	if n, err := unix.Fgetxattr(fd, "system.posix_acl_access", nil); n > 0 || err != nil && err != unix.ENODATA && err != unix.ENOTSUP {
		return nil, ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if len(data) > 1<<20 {
		return nil, ErrDenied
	}
	return data, err
}
