//go:build linux

package configmanager

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func writerFixture(t *testing.T) (*FileWriterServer, Resource, []byte) {
	t.Helper()
	path := writeTestINI(t)
	registry := testRegistry(path, false)
	registry.Resources[0].PrivilegedWrite = true
	registry.Resources[0].Backup = BackupPolicy{Enabled: true, Keep: 3}
	s, err := NewFileWriterServer(registry, filepath.Join(t.TempDir(), "state"), map[uint32]string{uint32(os.Getuid()): "application", 4000000000: "network"}, &recordingSink{})
	if err != nil {
		t.Fatal(err)
	}
	// Only fixtures bypass namespace ownership. The production constructor's
	// CLI/preflight always checks every directory down from filesystem root.
	s.validatePath = func(string) error { return nil }
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, registry.Resources[0], data
}

func TestPrivilegedWriterChangesOnlyDeclaredValueAndRetainsMetadata(t *testing.T) {
	s, r, before := writerFixture(t)
	if err := unix.Setxattr(r.Path, "user.safeterm-test", []byte("keep me"), 0); err != nil {
		t.Fatal(err)
	}
	// Linux ACL xattr: owner rw, named uid rw, owning group r, mask rw, other -.
	acl := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(acl, 2)
	entries := [][3]uint32{{1, 6, 0xffffffff}, {2, 6, uint32(os.Getuid() + 12345)}, {4, 4, 0xffffffff}, {16, 6, 0xffffffff}, {32, 0, 0xffffffff}}
	for i, e := range entries {
		o := 4 + i*8
		binary.LittleEndian.PutUint16(acl[o:], uint16(e[0]))
		binary.LittleEndian.PutUint16(acl[o+2:], uint16(e[1]))
		binary.LittleEndian.PutUint32(acl[o+4:], e[2])
	}
	if err := unix.Setxattr(r.Path, "system.posix_acl_access", acl, 0); err != nil {
		t.Fatal(err)
	}
	metadata, err := captureMetadata(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	p := Patch{ResourceID: r.ID, Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: contentVersion(before)}
	want, err := renderWriterPatch(r, before, p)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.commit(uint32(os.Getuid()), p, contentVersion(want))
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, want) {
		t.Fatal("unexpected file bytes")
	}
	got, err := captureMetadata(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !metadataEqual(metadata, got) {
		t.Fatal("owner/mode/ACL/xattrs changed")
	}
	backupInfo, err := os.Stat(filepath.Join(s.state, token))
	if err != nil || backupInfo.Mode().Perm() != 0600 {
		t.Fatal("backup is not private")
	}
	if err := s.restore(uint32(os.Getuid()), r.ID, token, contentVersion(after)); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(r.Path)
	if !bytes.Equal(restored, before) {
		t.Fatal("rollback did not restore exact comments and formatting")
	}
}

func TestWriterRejectsCrossProfileReadonlyInjectedAndStaleRequests(t *testing.T) {
	s, r, before := writerFixture(t)
	good := Patch{ResourceID: r.ID, Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: contentVersion(before)}
	for _, tc := range []struct {
		name     string
		uid      uint32
		patch    Patch
		expected string
	}{
		{"profile", 4000000000, good, strings.Repeat("0", 64)},
		{"unknown", uint32(os.Getuid()), Patch{ResourceID: "other", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: good.BaseVersion}, strings.Repeat("0", 64)},
		{"field", uint32(os.Getuid()), Patch{ResourceID: r.ID, Section: "cats", Key: "not_registered", Value: "100", BaseVersion: good.BaseVersion}, strings.Repeat("0", 64)},
		{"range", uint32(os.Getuid()), Patch{ResourceID: r.ID, Section: "cats", Key: "Cat_Port", Value: "999999", BaseVersion: good.BaseVersion}, strings.Repeat("0", 64)},
		{"preview", uint32(os.Getuid()), good, strings.Repeat("0", 64)},
		{"stale", uint32(os.Getuid()), Patch{ResourceID: r.ID, Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: strings.Repeat("0", 64)}, strings.Repeat("0", 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.commit(tc.uid, tc.patch, tc.expected); err == nil {
				t.Fatal("request accepted")
			}
			after, _ := os.ReadFile(r.Path)
			if !bytes.Equal(before, after) {
				t.Fatal("rejected request changed target")
			}
		})
	}
	r.Fields[1].ReadOnly = true
	s.resources[r.ID] = r
	if _, err := s.commit(uint32(os.Getuid()), good, strings.Repeat("0", 64)); err == nil {
		t.Fatal("read-only field accepted")
	}
	if err := s.restore(uint32(os.Getuid()), r.ID, "../foreign.json", good.BaseVersion); err == nil {
		t.Fatal("traversal accepted")
	}
}

func TestINILargeCommentedFileOnlyChangesSelectedValue(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		var b strings.Builder
		for section := 0; section < 40; section++ {
			fmt.Fprintf(&b, "; section note %d%s[Section_%d] ; header comment%s", section, ending, section, ending)
			for key := 0; key < 10; key++ {
				fmt.Fprintf(&b, "# keep key %d%s\tKey_%d = 'value_%d'   ; untouched note [x] # %s", key, ending, key, key, ending)
			}
		}
		original := b.String()
		doc, err := parseINI([]byte(original))
		if err != nil {
			t.Fatal(err)
		}
		if err := doc.Set("Section_23", "Key_7", "updated"); err != nil {
			t.Fatal(err)
		}
		got, _ := doc.Render()
		prefix := strings.Index(original, "[Section_23]")
		position := prefix + strings.Index(original[prefix:], "'value_7'")
		want := original[:position] + "'updated'" + original[position+len("'value_7'"):]
		if string(got) != want {
			t.Fatal("unrelated comments/sections/spacing/line endings changed")
		}
	}
	for _, value := range []string{"ok\n[Injected]\nA=1", "ok\rB=1", "ok\x00bad", "oops # inline", "'quoted'", " leading"} {
		doc, _ := parseINI([]byte("[s]\nk=value\n"))
		if err := doc.Set("s", "k", value); err == nil {
			t.Fatalf("ambiguous INI value accepted: %q", value)
		}
	}
}

func TestWriterIPCAuthenticatesServerAndSeparatesProfiles(t *testing.T) {
	s, r, before := writerFixture(t)
	socket := filepath.Join(t.TempDir(), "writer.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, socket) }()
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	client := &FileWriterClient{Socket: socket, UID: uint32(os.Getuid())}
	data, err := client.Read(r.ID)
	if err != nil || !bytes.Equal(data, before) {
		t.Fatalf("read: %v", err)
	}
	client.UID++
	if _, err := client.Read(r.ID); err == nil {
		t.Fatal("untrusted writer accepted")
	}
	client.UID = uint32(os.Getuid())
	p := Patch{ResourceID: r.ID, Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: contentVersion(before)}
	rendered, err := renderWriterPatch(r, before, p)
	if err != nil {
		t.Fatal(err)
	}
	token, err := client.Commit(p, contentVersion(rendered))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Restore(r.ID, token, contentVersion(rendered)); err != nil {
		t.Fatal(err)
	}
	for _, frame := range []string{
		`{"version":1,"action":"read","resource":"cats-options","path":"/etc/shadow"}`,
		`{"version":2,"version":1,"action":"read","resource":"cats-options"}`,
		`{"version":1,"action":"read","resource":"cats-options"} {}`,
		`{"version":1,"action":"read","resource":` + strings.Repeat("[", 11000) + `"cats-options"` + strings.Repeat("]", 11000) + `}`,
	} {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(time.Second))
		io.WriteString(conn, frame+"\n")
		response, err := io.ReadAll(conn)
		conn.Close()
		if len(response) > 0 {
			t.Fatalf("ambiguous wire input accepted: %q", response)
		}
	}
	// The wire format cannot carry a replacement path or complete document.
	if result := s.dispatch(uint32(os.Getuid()), writerRequest{Version: 1, Action: "commit", Resource: "/etc/shadow", Patch: &p, Expected: contentVersion(rendered)}); result.Error == "" {
		t.Fatal("unexpected request shape accepted")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPrivilegedWriterRejectsUnsafeNamespacesAndLinks(t *testing.T) {
	for _, path := range []string{"/tmp/app/options.ini", "/etc/sudoers.d/test", "/etc/passwd", "/etc/systemd/system/test.service", "/etc/safeterm/access/ssh/users.json", "/var/lib/safeterm-config-writer/state.json", "/etc/options.ini", "/path/../options.ini", "/path/to/%h.ini"} {
		if err := ValidateWriterPath(path); err == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	s, r, before := writerFixture(t)
	original := r.Path + ".original"
	if err := os.Rename(r.Path, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, r.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.read(uint32(os.Getuid()), r.ID); err == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(r.Path)
	if err := os.Link(original, r.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.read(uint32(os.Getuid()), r.ID); err == nil {
		t.Fatal("hardlink accepted")
	}
	got, _ := os.ReadFile(original)
	if !bytes.Equal(got, before) {
		t.Fatal("unsafe read changed file")
	}
}

func TestWriterAuditFailureAndStaleRollbackLeaveFileUntouched(t *testing.T) {
	s, r, before := writerFixture(t)
	p := Patch{ResourceID: r.ID, Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: contentVersion(before)}
	rendered, err := renderWriterPatch(r, before, p)
	if err != nil {
		t.Fatal(err)
	}
	s.sink = &recordingSink{fail: true}
	if _, err := s.commit(uint32(os.Getuid()), p, contentVersion(rendered)); err == nil {
		t.Fatal("write proceeded without audit")
	}
	got, _ := os.ReadFile(r.Path)
	if !bytes.Equal(got, before) {
		t.Fatal("unaudited write changed target")
	}
	s.sink = &recordingSink{}
	token, err := s.commit(uint32(os.Getuid()), p, contentVersion(rendered))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.restore(uint32(os.Getuid()), r.ID, token, p.BaseVersion); err == nil {
		t.Fatal("stale rollback accepted")
	}
	got, _ = os.ReadFile(r.Path)
	if !bytes.Equal(got, rendered) {
		t.Fatal("stale rollback changed file")
	}
}

func TestWriterBatchIsOneCommitAndRejectsUnauthorizedFields(t *testing.T) {
	s, r, before := writerFixture(t)
	p := Patch{ResourceID: r.ID, BaseVersion: contentVersion(before), Changes: []FieldChange{{Section: "cats", Key: "Cat_Port", Value: "100"}, {Section: "cats", Key: "Password", Value: "new-secret"}}}
	rendered, err := renderWriterPatch(r, before, p)
	if err != nil {
		t.Fatal(err)
	}
	bad := p
	bad.Changes = append(append([]FieldChange(nil), p.Changes...), FieldChange{Section: "cats", Key: "Immutable", Value: "bad"})
	if _, err = s.commit(uint32(os.Getuid()), bad, contentVersion(rendered)); err == nil {
		t.Fatal("unauthorized batch accepted")
	}
	after, _ := os.ReadFile(r.Path)
	if !bytes.Equal(after, before) {
		t.Fatal("failed batch partially committed")
	}
	token, err := s.commit(uint32(os.Getuid()), p, contentVersion(rendered))
	if err != nil {
		t.Fatal(err)
	}
	names, _ := s.snapshots(r.ID)
	if len(names) != 1 {
		t.Fatalf("backups=%v", names)
	}
	after, _ = os.ReadFile(r.Path)
	if !bytes.Equal(after, rendered) {
		t.Fatal("batch not committed")
	}
	if err = s.restore(uint32(os.Getuid()), r.ID, token, contentVersion(after)); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(r.Path)
	if !bytes.Equal(after, before) {
		t.Fatal("batch not restored")
	}
}
