//go:build linux

package passwordchange

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

type credentialFile struct {
	data     []byte
	mode     os.FileMode
	uid, gid uint32
}

func readCredential(root, relative string) (credentialFile, error) {
	path := filepath.Join(root, relative)
	expected := uint32(0)
	if root != "/" {
		expected = uint32(os.Geteuid())
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return credentialFile{}, err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || st.Uid != expected || info.Mode().Perm()&0022 != 0 {
			return credentialFile{}, fmt.Errorf("unsafe password directory")
		}
		if dir == filepath.Clean(root) {
			break
		}
		if dir == "/" {
			return credentialFile{}, ErrDenied
		}
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return credentialFile{}, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return credentialFile{}, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Size() > 1<<20 || info.Mode().Perm()&0027 != 0 || st.Uid != expected || st.Nlink != 1 {
		return credentialFile{}, ErrDenied
	}
	if n, err := unix.Fgetxattr(fd, "system.posix_acl_access", nil); n > 0 || err != nil && err != unix.ENODATA && err != unix.ENOTSUP {
		return credentialFile{}, ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return credentialFile{}, ErrDenied
	}
	return credentialFile{data: data, mode: info.Mode().Perm(), uid: st.Uid, gid: st.Gid}, nil
}
func replaceCredential(root, relative string, old credentialFile, data []byte) error {
	current, err := readCredential(root, relative)
	defer clear(current.data)
	if err != nil || !bytes.Equal(current.data, old.data) || current.mode != old.mode || current.uid != old.uid || current.gid != old.gid {
		return fmt.Errorf("password file changed; retry the password change")
	}
	path := filepath.Join(root, relative)
	f, err := os.CreateTemp(filepath.Dir(path), ".password-change-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(old.mode); err == nil {
		err = f.Chown(int(old.uid), int(old.gid))
	}
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return ErrChanged
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return ErrChanged
	}
	return nil
}
