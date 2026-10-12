package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndCompletion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help"}, &out, &out); err != nil || !strings.Contains(out.String(), "termctl catalog") {
		t.Fatalf("help output=%q err=%v", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"completion", "bash"}, &out, &out); err != nil || !strings.Contains(out.String(), "complete -F _termctl_complete termctl") {
		t.Fatalf("completion output=%q err=%v", out.String(), err)
	}
}

func TestUserListAndSSHAuthUpdate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "console"), 0o755); err != nil {
		t.Fatal(err)
	}
	sshData := `{"users":[{"user_id":"alice","roles":["viewer"],"password_hash":"$2a$12$C6UzMDM.H6dfI/f/IKcEe.7W8QbZ0pS8Qw6tYfYfHjS2K7S5m8x2K"}]}`
	if err := os.WriteFile(filepath.Join(dir, "ssh", "users.json"), []byte(sshData), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "console", "users.json"), []byte(`{"users":[{"os_user":"safeterm","user_id":"console","roles":["viewer"]}]}`), 0o640); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := userCommand(dir, []string{"list"}, &out); err != nil || !strings.Contains(out.String(), "ssh/alice") || !strings.Contains(out.String(), "console/console") {
		t.Fatalf("list=%q err=%v", out.String(), err)
	}
}

func TestCatalogListUsesShippedConfig(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--config-dir", "../../configs", "catalog", "list"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ping") || !strings.Contains(out.String(), "trace_filtered") {
		t.Fatalf("catalog output missing new operations: %q", out.String())
	}
}

func TestSelectedServicesUsesSafeBulkOrder(t *testing.T) {
	started, err := selectedServices("all", "start")
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != len(serviceOrder) || started[0] != "audit" || started[len(started)-1] != "ssh" {
		t.Fatalf("start order=%v", started)
	}
	stopped, err := selectedServices("all", "stop")
	if err != nil {
		t.Fatal(err)
	}
	if stopped[0] != "ssh" || stopped[len(stopped)-1] != "audit" {
		t.Fatalf("stop order=%v", stopped)
	}
	if _, err := selectedServices("unknown", "status"); err == nil {
		t.Fatal("unknown service was accepted")
	}
}
