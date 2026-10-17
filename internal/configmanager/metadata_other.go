//go:build !linux

package configmanager

import "os"

type fileMetadata struct{ Mode os.FileMode }

func captureMetadata(path string) (fileMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileMetadata{}, err
	}
	return fileMetadata{info.Mode().Perm()}, nil
}
func applyMetadata(file *os.File, m fileMetadata) error { return file.Chmod(m.Mode) }

func sameMetadata(a, b fileMetadata) bool { return a == b }
