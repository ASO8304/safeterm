//go:build linux

package configmanager

import (
	"bytes"
	"fmt"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

type fileMetadata struct {
	UID, GID   int
	Mode       uint32
	Attributes map[string][]byte
}

func captureMetadata(path string) (fileMetadata, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fileMetadata{}, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileMetadata{}, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Mode&07000 != 0 {
		return fileMetadata{}, fmt.Errorf("configmanager: unsafe file metadata")
	}
	m := fileMetadata{UID: int(stat.Uid), GID: int(stat.Gid), Mode: stat.Mode & 0777, Attributes: map[string][]byte{}}
	n, err := unix.Flistxattr(fd, nil)
	if err == unix.ENOTSUP {
		return m, nil
	}
	if err != nil || n > 65536 {
		return m, fmt.Errorf("configmanager: cannot safely enumerate extended attributes: %v", err)
	}
	list := make([]byte, n)
	n, err = unix.Flistxattr(fd, list)
	if err == nil && n > len(list) {
		err = fmt.Errorf("extended attributes changed during read")
	}
	if err != nil {
		return m, err
	}
	total := 0
	for _, name := range bytes.Split(list[:n], []byte{0}) {
		if len(name) == 0 {
			continue
		}
		size, err := unix.Fgetxattr(fd, string(name), nil)
		if err != nil || size > 65536 {
			return m, fmt.Errorf("configmanager: cannot safely read extended attribute")
		}
		value := make([]byte, size)
		size, err = unix.Fgetxattr(fd, string(name), value)
		if err == nil && size > len(value) {
			err = fmt.Errorf("extended attribute changed during read")
		}
		if err != nil {
			return m, err
		}
		total += size
		if total > 262144 {
			return m, fmt.Errorf("configmanager: extended attributes exceed limit")
		}
		if string(name) == "security.capability" || string(name) == "security.ima" || string(name) == "security.evm" {
			return m, fmt.Errorf("configmanager: executable capabilities or content signatures cannot be preserved across config edits")
		}
		m.Attributes[string(name)] = value[:size]
	}
	return m, nil
}

func applyMetadata(file *os.File, m fileMetadata) error {
	if err := file.Chown(m.UID, m.GID); err != nil {
		return fmt.Errorf("configmanager: preserve file owner: %w", err)
	}
	fd := int(file.Fd())
	// Remove inherited attributes/ACLs which were absent on the original file.
	n, err := unix.Flistxattr(fd, nil)
	if err == unix.ENOTSUP && len(m.Attributes) == 0 {
		return file.Chmod(os.FileMode(m.Mode))
	}
	if err != nil || n > 65536 {
		return fmt.Errorf("configmanager: inspect replacement attributes: %v", err)
	}
	list := make([]byte, n)
	n, err = unix.Flistxattr(fd, list)
	if err == nil && n > len(list) {
		err = fmt.Errorf("extended attributes changed during read")
	}
	if err != nil {
		return err
	}
	for _, name := range bytes.Split(list[:n], []byte{0}) {
		if len(name) > 0 {
			if _, ok := m.Attributes[string(name)]; !ok {
				if err := unix.Fremovexattr(fd, string(name)); err != nil {
					return err
				}
			}
		}
	}
	if err := file.Chmod(os.FileMode(m.Mode)); err != nil {
		return err
	}
	for name, value := range m.Attributes {
		if err := unix.Fsetxattr(fd, name, value, 0); err != nil {
			return fmt.Errorf("configmanager: preserve extended attribute %q: %w", name, err)
		}
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&0777 != m.Mode || int(stat.Uid) != m.UID || int(stat.Gid) != m.GID {
		return fmt.Errorf("configmanager: metadata preservation failed")
	}
	actual, err := captureMetadata(file.Name())
	if err != nil {
		return err
	}
	if !sameMetadata(actual, m) {
		return fmt.Errorf("configmanager: extended metadata verification failed")
	}
	return nil
}

func sameMetadata(a, b fileMetadata) bool { return reflect.DeepEqual(a, b) }
