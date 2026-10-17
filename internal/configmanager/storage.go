package configmanager

import (
	"fmt"
	"os"
)

// FileWriter accepts semantic resource/field operations only. Paths, replacement
// documents and executable commands never cross the privileged writer protocol.
type FileWriter interface {
	Read(string) ([]byte, error)
	Commit(Patch, string) (string, error)
	Restore(string, string, string) error
	Latest(string) (string, error)
}

func (m *Manager) readFile(resource Resource) ([]byte, os.FileInfo, error) {
	if !resource.PrivilegedWrite {
		return readManagedFile(resource.Path)
	}
	if m.writer == nil {
		return nil, nil, fmt.Errorf("configmanager: resource requires the privileged file writer")
	}
	data, err := m.writer.Read(resource.ID)
	return data, nil, err
}
func (m *Manager) commitFile(r Resource, p Patch, rendered, current []byte, info os.FileInfo) (string, error) {
	if r.PrivilegedWrite {
		return m.writer.Commit(p, contentVersion(rendered))
	}
	return commitReplacement(r, rendered, current, info)
}
func (m *Manager) restoreFile(r Resource, backup, expected string) error {
	if r.PrivilegedWrite {
		return m.writer.Restore(r.ID, backup, expected)
	}
	data, _, err := readManagedFile(r.Path)
	if err != nil {
		return err
	}
	if contentVersion(data) != expected {
		return fmt.Errorf("configmanager: rollback version conflict")
	}
	return restoreBackup(r, backup)
}
func (m *Manager) latestFileBackup(r Resource) (string, error) {
	if r.PrivilegedWrite {
		return m.writer.Latest(r.ID)
	}
	return latestBackup(r)
}
