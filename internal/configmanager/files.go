package configmanager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// readManagedFile rejects links and verifies that the object opened is the
// same regular file that was inspected. This prevents a registry entry from
// silently redirecting the manager to another target.
func readManagedFile(path string) ([]byte, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("configmanager: inspect managed file: %w", err)
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > maxDocumentBytes {
		return nil, nil, fmt.Errorf("configmanager: managed path must be a regular non-symlink file no larger than %d bytes", maxDocumentBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("configmanager: open managed file: %w", err)
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("configmanager: managed file changed while opening")
	}
	if links, ok := fileLinkCount(after); ok && links != 1 {
		return nil, nil, fmt.Errorf("configmanager: managed file must not have hard links")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil || len(data) > maxDocumentBytes {
		return nil, nil, fmt.Errorf("configmanager: read managed file: size or I/O limit exceeded")
	}
	return data, after, nil
}

func commitReplacement(resource Resource, rendered, expected []byte, info os.FileInfo) (string, error) {
	current, currentInfo, err := readManagedFile(resource.Path)
	if err != nil {
		return "", err
	}
	if contentVersion(current) != contentVersion(expected) || currentInfo.Mode().Perm() != info.Mode().Perm() {
		return "", fmt.Errorf("configmanager: managed file changed before commit")
	}
	document, err := parseDocument(resource.Format, rendered)
	if err != nil {
		return "", err
	}
	if err := validateCompleteDocument(resource, document); err != nil {
		return "", err
	}
	backup := ""
	if resource.Backup.Enabled {
		backup, err = writeBackup(resource, current, currentInfo)
		if err != nil {
			return "", err
		}
		if err := pruneBackups(resource); err != nil {
			_ = os.Remove(backup)
			return "", err
		}
	}
	if err := atomicWrite(resource.Path, rendered, currentInfo); err != nil {
		if backup != "" {
			_ = os.Remove(backup)
		}
		return "", err
	}
	return backup, nil
}

func atomicWrite(path string, data []byte, info os.FileInfo) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".safeterm-write-*")
	if err != nil {
		return fmt.Errorf("configmanager: create temporary file: %w", err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("configmanager: preserve file mode: %w", err)
	}
	if uid, gid, ok := fileOwnerIDs(info); ok {
		if err := temp.Chown(uid, gid); err != nil {
			return fmt.Errorf("configmanager: preserve file owner: %w", err)
		}
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("configmanager: write temporary file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("configmanager: sync temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("configmanager: close temporary file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("configmanager: atomically replace managed file: %w", err)
	}
	committed = true
	if err := syncDirectory(directory); err != nil {
		return err
	}
	return nil
}

func writeBackup(resource Resource, data []byte, info os.FileInfo) (string, error) {
	name := fmt.Sprintf(".safeterm-%s-%d-%s.bak", resource.ID, time.Now().UTC().UnixNano(), contentVersion(data)[:12])
	path := filepath.Join(filepath.Dir(resource.Path), name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return "", fmt.Errorf("configmanager: create backup: %w", err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if uid, gid, present := fileOwnerIDs(info); present {
		if err := file.Chown(uid, gid); err != nil {
			return "", fmt.Errorf("configmanager: preserve backup owner: %w", err)
		}
	}
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("configmanager: write backup: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("configmanager: sync backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("configmanager: close backup: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func backupPaths(resource Resource) ([]string, error) {
	entries, err := os.ReadDir(filepath.Dir(resource.Path))
	if err != nil {
		return nil, fmt.Errorf("configmanager: list backups: %w", err)
	}
	prefix := ".safeterm-" + resource.ID + "-"
	paths := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".bak") && entry.Type().IsRegular() {
			paths = append(paths, filepath.Join(filepath.Dir(resource.Path), entry.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths, nil
}

func latestBackup(resource Resource) (string, error) {
	paths, err := backupPaths(resource)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("configmanager: no backup is available")
	}
	return paths[0], nil
}

func pruneBackups(resource Resource) error {
	paths, err := backupPaths(resource)
	if err != nil {
		return err
	}
	if len(paths) <= resource.Backup.Keep {
		return nil
	}
	for _, path := range paths[resource.Backup.Keep:] {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("configmanager: enforce backup retention: %w", err)
		}
	}
	return syncDirectory(filepath.Dir(resource.Path))
}

func restoreBackup(resource Resource, backup string) error {
	cleanDirectory := filepath.Clean(filepath.Dir(resource.Path))
	if filepath.Clean(filepath.Dir(backup)) != cleanDirectory || !strings.HasPrefix(filepath.Base(backup), ".safeterm-"+resource.ID+"-") {
		return fmt.Errorf("configmanager: invalid backup path")
	}
	data, _, err := readManagedFile(backup)
	if err != nil {
		return err
	}
	document, err := parseDocument(resource.Format, data)
	if err != nil {
		return err
	}
	if err := validateCompleteDocument(resource, document); err != nil {
		return err
	}
	_, info, err := readManagedFile(resource.Path)
	if err != nil {
		return err
	}
	return atomicWrite(resource.Path, data, info)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("configmanager: open parent directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("configmanager: sync parent directory: %w", err)
	}
	return nil
}
