//go:build !linux

package broker

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

func readSecurityFile(path, description string, maxBytes int64, forbiddenPermissions os.FileMode) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("broker: inspect %s: %w", description, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("broker: %s must be a regular file no larger than %d bytes", description, maxBytes)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&forbiddenPermissions != 0 {
		return nil, fmt.Errorf("broker: %s has insecure permissions", description)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("broker: open %s: %w", description, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("broker: read %s: %w", description, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("broker: %s exceeds %d bytes", description, maxBytes)
	}
	return data, nil
}
