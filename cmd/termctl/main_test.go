package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBulkRestartStopsDependentsBeforeStartingDependencies(t *testing.T) {
	var out bytes.Buffer
	var calls []string
	err := serviceCommandWithRunner([]string{"restart", "all"}, &out, func(action, unit string) error {
		calls = append(calls, action+" "+unit)
		return nil
	})
	n := len(serviceOrder)
	if err != nil || len(calls) != 3*n {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	for i, name := range serviceOrder {
		if calls[n-1-i] != "stop "+serviceUnits[name] || calls[n+i] != "reset-failed "+serviceUnits[name] || calls[2*n+i] != "start "+serviceUnits[name] {
			t.Fatalf("incorrect restart ordering: %v", calls)
		}
	}
}

func TestBulkRestartAbortsAfterFailure(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	err := serviceCommandWithRunner([]string{"restart", "all"}, &out, func(action, unit string) error {
		calls++
		return errors.New("failed")
	})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestBashCompletionActuallyCompletes(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	var script bytes.Buffer
	if err := writeCompletion(&script, []string{"bash"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		words string
		index int
		want  string
	}{
		{"termctl ser", 1, "service"},
		{"termctl service re", 2, "restart"},
		{"termctl service restart config-n", 3, "config-network"},
		{"termctl user add s", 3, "ssh"},
		{"termctl catalog reg", 2, "regenerate-hashes"},
		{"termctl user add ssh --password", 4, "--password-hash-file"},
	} {
		code := script.String() + fmt.Sprintf("\nCOMP_WORDS=(%s); COMP_CWORD=%d; _termctl_complete; printf '%%s\\n' \"${COMPREPLY[@]}\"", tc.words, tc.index)
		out, err := exec.Command(bash, "--noprofile", "--norc", "-c", code).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != tc.want {
			t.Errorf("%s: output=%q err=%v want=%s", tc.words, out, err, tc.want)
		}
	}
}

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

func TestLifecycleRejectsInvalidInput(t *testing.T) {
	var out bytes.Buffer
	for _, args := range [][]string{
		{"rollback", "extra"}, {"uninstall", "--purge"}, {"update"},
		{"update", t.TempDir()},
	} {
		if err := run(args, &out, &out); err == nil {
			t.Fatalf("invalid lifecycle input accepted: %v", args)
		}
	}
	if err := run([]string{"help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"termctl update", "termctl rollback", "termctl uninstall"} {
		if !strings.Contains(out.String(), command) {
			t.Fatalf("missing help for %s", command)
		}
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
