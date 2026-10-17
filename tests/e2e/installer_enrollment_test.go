//go:build linux

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerGuidedEnrollmentAndUnattendedBoundary(t *testing.T) {
	support, err := filepath.Abs("../../deploy/package/install-support.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input, setup, want string
		fail                     bool
	}{
		{"default user", "\n", "", "RESULT:admin:true:", false},
		{"custom user", "demo_admin\n", "", "RESULT:demo_admin:true:", false},
		{"invalid user", "bad user\n", "", "Invalid SafeTerm username", true},
		{"explicit user", "", "ssh_user=automated; ssh_user_set=true", "RESULT:automated:true:", false},
		{"unattended missing credential", "", "non_interactive=true", "Fresh installation needs credentials", true},
		{"redirected input", "", "safeterm_has_terminal() { return 1; }", "Fresh installation needs credentials", true},
		{"provided hash", "", "non_interactive=true; ssh_password_hash_file=/secure/hash", "RESULT:admin:false:/secure/hash", false},
		{"provided public key", "", "non_interactive=true; ssh_public_key=/secure/key.pub", "RESULT:admin:false:", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			bin := filepath.Join(work, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			// Password terminal handling is tested in cmd/password-hash. This
			// recorder isolates installer orchestration without touching a host.
			if err := os.WriteFile(filepath.Join(bin, "safeterm-password-hash"), []byte("#!/bin/sh\n[ \"$1\" = -output ] || exit 1\numask 077\nprintf 'bcrypt-test-hash\\n' > \"$2\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			code := `set -eu
. "$SUPPORT"
work="$FIXTURE"; payload="$FIXTURE"; ssh_user=admin; ssh_user_set=false
non_interactive=false; ssh_password_hash_file=; ssh_public_key=; bootstrap_console=false
safeterm_has_terminal() { return 0; }
` + tc.setup + `
safeterm_enroll
printf 'RESULT:%s:%s:%s\n' "$ssh_user" "$bootstrap_console" "$ssh_password_hash_file"
`
			cmd := exec.Command("sh", "-c", code)
			cmd.Env = append(os.Environ(), "SUPPORT="+support, "FIXTURE="+work)
			cmd.Stdin = strings.NewReader(tc.input)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail || !strings.Contains(string(out), tc.want) {
				t.Fatalf("output=%s error=%v", out, err)
			}
			if tc.fail || tc.setup != "" && tc.name != "explicit user" {
				return
			}
			info, err := os.Stat(filepath.Join(work, "bootstrap.hash"))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("unsafe temporary hash: %v %v", info, err)
			}
		})
	}
}

func TestInstallerPasswordFailureCanBeCancelled(t *testing.T) {
	support, err := filepath.Abs("../../deploy/package/install-support.sh")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Mkdir(filepath.Join(work, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "bin/safeterm-password-hash"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `set -eu
. "$SUPPORT"
work="$FIXTURE"; payload="$FIXTURE"; ssh_user=admin; ssh_user_set=true
non_interactive=false; ssh_password_hash_file=; ssh_public_key=; bootstrap_console=false
safeterm_has_terminal() { return 0; }
safeterm_enroll
echo UNEXPECTED_SUCCESS
`)
	cmd.Env = append(os.Environ(), "SUPPORT="+support, "FIXTURE="+work)
	cmd.Stdin = strings.NewReader("n\n")
	out, err := cmd.CombinedOutput()
	if err == nil || strings.Contains(string(out), "UNEXPECTED_SUCCESS") {
		t.Fatalf("cancelled enrollment continued: %s %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(work, "bootstrap.hash")); !os.IsNotExist(err) {
		t.Fatal("cancel left bootstrap hash")
	}
}

func TestInstallerDependencyPolicyNeverMutatesDuringDryRunOrImplicitAutomation(t *testing.T) {
	support, err := filepath.Abs("../../deploy/package/install-support.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, setup, want string
		fail, apt         bool
	}{
		{"dry run", "dry_run=true", "Dry run does not install OS packages", true, false},
		{"unattended", "non_interactive=true", "--install-dependencies", true, false},
		{"redirected", "safeterm_has_terminal() { return 1; }", "--install-dependencies", true, false},
		{"guided", "", "APT:install -y --no-upgrade", false, true},
		{"explicit automatic", "non_interactive=true; install_dependencies=true", "APT:install -y --no-upgrade", false, true},
		{"repository failure", "export APT_FAIL=1", "provision prerequisites offline", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "apt-get"), []byte("#!/bin/sh\nif [ \"${APT_FAIL:-0}\" = 1 ]; then echo REPOSITORY_FAILURE; exit 1; fi\necho \"APT:$*\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", "-c", `set -eu
. "$SUPPORT"
dry_run=false; non_interactive=false; install_dependencies=false; checks=0
safeterm_has_terminal() { return 0; }
safeterm_find_missing() { checks=$((checks+1)); if [ "$checks" = 1 ]; then missing=demo; else missing=; fi; }
`+tc.setup+`
safeterm_dependencies
`)
			cmd.Env = append(os.Environ(), "SUPPORT="+support, "PATH="+bin+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail || !strings.Contains(string(out), tc.want) || strings.Contains(string(out), "APT:") != tc.apt {
				t.Fatalf("output=%s error=%v", out, err)
			}
		})
	}
}
