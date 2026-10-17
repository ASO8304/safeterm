//go:build linux

package configmanager

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWriterUnprivilegedManagerChild(t *testing.T) {
	if os.Getenv("SAFETERM_WRITER_ROOT_CHILD") != "1" {
		return
	}
	path := os.Getenv("SAFETERM_WRITER_ROOT_TARGET")
	if err := os.WriteFile(path, []byte("unauthorized"), 0600); err == nil {
		t.Fatal("unprivileged manager can write target directly")
	}
	registry := testRegistry(path, false)
	registry.Resources[0].PrivilegedWrite = true
	client := &FileWriterClient{Socket: os.Getenv("SAFETERM_WRITER_ROOT_SOCKET"), UID: 0}
	m, err := NewManager("application", registry, nil, nil, &recordingSink{}, ManagerOptions{FileWriter: client})
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{BrokerType: "ssh", UserID: "operator", Roles: []string{"viewer", "operator"}}
	read, err := m.Read(context.Background(), actor, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	p := Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "100", BaseVersion: read.Version}
	preview, err := m.Preview(context.Background(), actor, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), actor, preview.Token); err != nil {
		t.Fatal(err)
	}
}

// Run in CI as root against a disposable dedicated directory, never against a
// real application's configuration. The child has an actual non-root kernel UID.
func TestPrivilegedWriterRootIntegration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root integration runs in the isolated CI runner")
	}
	for _, owner := range []int{0, 12345} {
		t.Run(map[int]string{0: "root-owned", 12345: "service-owned"}[owner], func(t *testing.T) {
			dir, err := os.MkdirTemp("/var/lib", "safeterm-writer-test-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			if err := os.Chmod(dir, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "options.ini")
			before := []byte("; comment\r\n[cats] # keep section note\r\nCat_Path = \"/etc/cat\"\r\nCat_Port = \"99\" ; keep inline\r\nPassword = \"hidden\"\r\nImmutable = \"fixed\"\r\n")
			if err := os.WriteFile(path, before, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chown(path, owner, owner); err != nil {
				t.Fatal(err)
			}
			if owner != 0 {
				if err := os.Chmod(path, 0640); err != nil {
					t.Fatal(err)
				}
			}
			registry := testRegistry(path, false)
			registry.Resources[0].PrivilegedWrite = true
			state := filepath.Join(dir, "state")
			s, err := NewFileWriterServer(registry, state, map[uint32]string{65534: "application"}, &recordingSink{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := WriterUnitDropIn(registry, true); err != nil {
				t.Fatal(err)
			}
			socket := filepath.Join(dir, "writer.sock")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.Serve(ctx, socket) }()
			for i := 0; i < 200; i++ {
				if _, err := os.Stat(socket); err == nil {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err := os.Chown(socket, 0, 65534); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			binaryData, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			child := filepath.Join(dir, "child.test")
			if err := os.WriteFile(child, binaryData, 0555); err != nil {
				t.Fatal(err)
			}
			metadata, err := captureMetadata(path)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(child, "-test.run=^TestWriterUnprivilegedManagerChild$")
			cmd.Env = append(os.Environ(), "SAFETERM_WRITER_ROOT_CHILD=1", "SAFETERM_WRITER_ROOT_TARGET="+path, "SAFETERM_WRITER_ROOT_SOCKET="+socket)
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("unprivileged manager: %s %v", output, err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := bytes.Replace(before, []byte("\"99\""), []byte("\"100\""), 1)
			if !bytes.Equal(after, want) {
				t.Fatal("unrelated INI bytes changed")
			}
			got, err := captureMetadata(path)
			if err != nil || !metadataEqual(metadata, got) {
				t.Fatal("foreign ownership or metadata changed")
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
