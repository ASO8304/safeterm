//go:build linux

package broker

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

func readSecurityFile(path, description string, maxBytes int64, forbiddenPermissions os.FileMode) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("broker: open %s: %w", description, err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("broker: inspect %s: %w", description, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("broker: %s must be a regular file no larger than %d bytes", description, maxBytes)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		return nil, fmt.Errorf("broker: %s must not have hard links", description)
	}
	if info.Mode().Perm()&forbiddenPermissions != 0 {
		return nil, fmt.Errorf("broker: %s has insecure permissions", description)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("broker: read %s: %w", description, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("broker: %s exceeds %d bytes", description, maxBytes)
	}
	return data, nil
}
