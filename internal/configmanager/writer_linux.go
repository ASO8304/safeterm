//go:build linux

package configmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"safeterm/internal/audit"
)

const WriterSocket = "/run/safeterm-config-writer/writer.sock"
const WriterState = "/var/lib/safeterm-config-writer"

// ValidateWriterPath requires a root-controlled namespace. ACL write access on
// parent directories is rejected too: a delegated caller cannot swap targets.
func ValidateWriterPath(path string) error {
	for _, c := range path {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/_-.", c)) {
			return fmt.Errorf("configwriter: unsupported characters in target path")
		}
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("configwriter: path must be canonical")
	}
	for _, blocked := range []string{"/proc", "/sys", "/dev", "/run", "/tmp", "/var/tmp", "/root", "/home", "/usr", "/bin", "/sbin", "/lib", "/boot", "/etc/safeterm", "/etc/systemd", "/etc/ssh", "/etc/sudoers", "/etc/cron", "/etc/pam.d", "/etc/security", "/etc/polkit-1", "/etc/ld.so", "/var/lib/safeterm", "/var/lib/safeterm-config-writer", "/var/lib/safeterm-installer"} {
		if path == blocked || strings.HasPrefix(path, blocked+"/") || strings.HasPrefix(path, blocked+".") {
			return fmt.Errorf("configwriter: protected target")
		}
	}
	for _, blocked := range []string{"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/gshadow", "/etc/shells", "/etc/environment", "/etc/profile", "/etc/crontab", "/etc/fstab"} {
		if path == blocked {
			return fmt.Errorf("configwriter: protected target")
		}
	}
	parent := filepath.Dir(path)
	for _, broad := range []string{"/", "/etc", "/var", "/var/lib", "/var/log", "/opt", "/srv"} {
		if parent == broad {
			return fmt.Errorf("configwriter: resource needs a dedicated application directory")
		}
	}
	return validateWriterParents(parent)
}

// ValidateWriterRegistry checks the trust namespace before root loads policy.
func ValidateWriterRegistry() error { return validateWriterParents("/etc/safeterm/config-manager") }

func validateWriterParents(parent string) error {
	for dir := parent; ; dir = filepath.Dir(dir) {
		fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		var st unix.Stat_t
		err = unix.Fstat(fd, &st)
		if err == nil && (st.Uid != 0 || st.Mode&0022 != 0) {
			err = fmt.Errorf("configwriter: parent must be root-owned and not writable by other users")
		}
		// Named ACL permissions are reflected in the group mask bits above.
		unix.Close(fd)
		if err != nil {
			return err
		}
		if dir == "/" {
			break
		}
	}
	return nil
}

type writerSnapshot struct {
	Resource, Path string
	Data           []byte
	Metadata       fileMetadata
}

type FileWriterServer struct {
	resources    map[string]Resource
	peers        map[uint32]string
	state        string
	sink         audit.Sink
	mu           sync.Mutex
	validatePath func(string) error
}

func NewFileWriterServer(registry *Registry, state string, peers map[uint32]string, sink audit.Sink) (*FileWriterServer, error) {
	if err := ValidateRegistry(registry); err != nil {
		return nil, err
	}
	if sink == nil || len(peers) == 0 {
		return nil, fmt.Errorf("configwriter: audit and peer mappings required")
	}
	if err := os.MkdirAll(state, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(state)
	if err != nil {
		return nil, err
	}
	uid, ok := fileOwnerUID(info)
	if !info.IsDir() || !ok || uid != uint32(os.Geteuid()) || info.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("configwriter: unsafe state directory")
	}
	s := &FileWriterServer{resources: map[string]Resource{}, peers: peers, state: state, sink: sink, validatePath: ValidateWriterPath}
	for _, r := range registry.Resources {
		if r.PrivilegedWrite {
			if r.ValidatorID != "" {
				return nil, fmt.Errorf("configwriter: external validators are unsupported in privileged resources")
			}
			s.resources[r.ID] = r
		}
	}
	return s, nil
}

func (s *FileWriterServer) resource(uid uint32, id string) (Resource, error) {
	r, ok := s.resources[id]
	profile, trusted := s.peers[uid]
	if !ok || !trusted || profile != r.Profile {
		return Resource{}, fmt.Errorf("configwriter: resource denied")
	}
	if err := s.validatePath(r.Path); err != nil {
		return Resource{}, err
	}
	return r, nil
}

func (s *FileWriterServer) record(r Resource, action, outcome, base, next string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.sink.Record(ctx, audit.Event{SchemaVersion: audit.SchemaVersion, RequestID: randomID(), CatalogVersion: "file-writer-v1", Component: audit.ComponentConfigManager, Profile: "file-writer", Operation: action, Outcome: outcome, ResourceID: r.ID, UserID: "manager:" + r.Profile, BaseVersion: base, NewVersion: next, OccurredAt: time.Now().UTC()})
}

func (s *FileWriterServer) read(uid uint32, id string) ([]byte, error) {
	r, err := s.resource(uid, id)
	if err != nil {
		return nil, err
	}
	data, _, err := readManagedFile(r.Path)
	return data, err
}

func renderWriterPatch(r Resource, data []byte, p Patch) ([]byte, error) {
	if r.Access != AccessReadWrite || p.ResourceID != r.ID || contentVersion(data) != p.BaseVersion {
		return nil, fmt.Errorf("configwriter: write denied or version conflict")
	}
	var field *FieldSchema
	for i := range r.Fields {
		if r.Fields[i].Section == p.Section && r.Fields[i].Key == p.Key {
			field = &r.Fields[i]
			break
		}
	}
	if field == nil || field.ReadOnly {
		return nil, fmt.Errorf("configwriter: field denied")
	}
	doc, err := parseDocument(r.Format, data)
	if err != nil {
		return nil, err
	}
	if _, exists := doc.Get(p.Section, p.Key); !exists {
		return nil, fmt.Errorf("configwriter: missing field")
	}
	value, normalized, err := normalizeValue(*field, p.Value)
	if err != nil {
		return nil, err
	}
	if r.Format == FormatJSON {
		value = typedValueForJSON(value)
	}
	if err := doc.Set(p.Section, p.Key, value); err != nil {
		return nil, err
	}
	if err := validateCompleteDocument(r, doc); err != nil {
		return nil, err
	}
	rendered, err := doc.Render()
	if err != nil {
		return nil, err
	}
	reparsed, err := parseDocument(r.Format, rendered)
	if err != nil {
		return nil, err
	}
	if err := validateCompleteDocument(r, reparsed); err != nil {
		return nil, err
	}
	actual, exists := reparsed.Get(p.Section, p.Key)
	if !exists || scalarString(actual) != normalized {
		return nil, fmt.Errorf("configwriter: value changed during rendering")
	}
	return rendered, nil
}

func (s *FileWriterServer) snapshots(id string) ([]string, error) {
	entries, err := os.ReadDir(s.state)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), id+"--") && strings.HasSuffix(entry.Name(), ".json") && entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	return names, nil // ReadDir sorts chronologically prefixed snapshot names.
}

func (s *FileWriterServer) commit(uid uint32, p Patch, want string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.resource(uid, p.ResourceID)
	if err != nil {
		return "", err
	}
	current, info, err := readManagedFile(r.Path)
	if err != nil {
		return "", err
	}
	rendered, err := renderWriterPatch(r, current, p)
	if err != nil {
		return "", err
	}
	if contentVersion(rendered) != want || want == p.BaseVersion {
		return "", fmt.Errorf("configwriter: unexpected preview version")
	}
	metadata, err := captureMetadata(r.Path)
	if err != nil {
		return "", err
	}
	if err := s.record(r, "apply", "apply-authorized", p.BaseVersion, want); err != nil {
		return "", err
	}
	token := ""
	if r.Backup.Enabled {
		token = fmt.Sprintf("%s--%020d-%s.json", r.ID, time.Now().UnixNano(), randomID())
		data, err := json.Marshal(writerSnapshot{r.ID, r.Path, current, metadata})
		if err != nil {
			return "", err
		}
		file, err := os.OpenFile(filepath.Join(s.state, token), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		_, err = file.Write(data)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = syncDirectory(s.state)
		}
		if err != nil {
			_ = os.Remove(filepath.Join(s.state, token))
			return "", err
		}
	}
	if r.Backup.Enabled {
		names, err := s.snapshots(r.ID)
		if err != nil {
			_ = os.Remove(filepath.Join(s.state, token))
			return "", err
		}
		for len(names) > r.Backup.Keep {
			if err := os.Remove(filepath.Join(s.state, names[0])); err != nil {
				_ = os.Remove(filepath.Join(s.state, token))
				return "", err
			}
			names = names[1:]
		}
	}
	// Recheck before replacement. No untrusted process can rename entries in
	// the root-controlled directory, and stale application edits are refused.
	now, nowInfo, err := readManagedFile(r.Path)
	if err != nil || !os.SameFile(info, nowInfo) || contentVersion(now) != p.BaseVersion {
		if token != "" {
			_ = os.Remove(filepath.Join(s.state, token))
		}
		return "", fmt.Errorf("configwriter: target changed")
	}
	if err := atomicWrite(r.Path, rendered, info); err != nil {
		if token != "" {
			_ = os.Remove(filepath.Join(s.state, token))
		}
		return "", err
	}
	if err := s.record(r, "apply", "applied", p.BaseVersion, want); err != nil {
		// Roll back the exact original bytes if completion cannot be audited.
		_, undoInfo, readErr := readManagedFile(r.Path)
		if readErr != nil {
			return "", readErr
		}
		if undoErr := atomicWrite(r.Path, current, undoInfo); undoErr != nil {
			return "", fmt.Errorf("configwriter: audit failed and recovery failed: %w", undoErr)
		}
		return "", err
	}
	return token, nil
}

func (s *FileWriterServer) restore(uid uint32, id, token, expected string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.resource(uid, id)
	if err != nil {
		return err
	}
	if r.Access != AccessReadWrite || !r.Backup.Enabled || filepath.Base(token) != token || !strings.HasPrefix(token, id+"--") || !strings.HasSuffix(token, ".json") {
		return fmt.Errorf("configwriter: backup denied")
	}
	path := filepath.Join(s.state, token)
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	uid, ok := fileOwnerUID(info)
	links, _ := fileLinkCount(info)
	if !ok || uid != uint32(os.Geteuid()) || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || links != 1 || info.Size() > 6<<20 {
		file.Close()
		return fmt.Errorf("configwriter: unsafe snapshot")
	}
	data, err := io.ReadAll(io.LimitReader(file, 6<<20+1))
	file.Close()
	if err != nil || len(data) > 6<<20 {
		return fmt.Errorf("configwriter: invalid snapshot")
	}
	var snapshot writerSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	if snapshot.Resource != id || snapshot.Path != r.Path {
		return fmt.Errorf("configwriter: backup target mismatch")
	}
	doc, err := parseDocument(r.Format, snapshot.Data)
	if err != nil {
		return err
	}
	if err := validateCompleteDocument(r, doc); err != nil {
		return err
	}
	current, currentInfo, err := readManagedFile(r.Path)
	if err != nil {
		return err
	}
	if contentVersion(current) != expected {
		return fmt.Errorf("configwriter: version conflict")
	}
	metadata, err := captureMetadata(r.Path)
	if err != nil {
		return err
	}
	if !metadataEqual(metadata, snapshot.Metadata) {
		return fmt.Errorf("configwriter: metadata changed since backup")
	}
	if err := s.record(r, "rollback", "rollback-authorized", expected, contentVersion(snapshot.Data)); err != nil {
		return err
	}
	if err := atomicWrite(r.Path, snapshot.Data, currentInfo); err != nil {
		return err
	}
	return s.record(r, "rollback", "rollback-succeeded", expected, contentVersion(snapshot.Data))
}

func metadataEqual(a, b fileMetadata) bool { return reflect.DeepEqual(a, b) }
