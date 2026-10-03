//go:build linux

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TEST-DEPLOY-003: REQ-015 AC-3, AC-5, and AC-7.
func TestDeploymentScriptsAreExecutableValidShellAndComplete(t *testing.T) {
	repository := repositoryRoot(t)
	scripts := []string{
		filepath.Join(repository, "deploy", "scripts", "build-bundle.sh"),
		filepath.Join(repository, "deploy", "scripts", "install.sh"),
		filepath.Join(repository, "deploy", "demo", "run-demo.sh"),
		filepath.Join(repository, "deploy", "package", "create-package.sh"),
		filepath.Join(repository, "deploy", "package", "install-package.sh"),
	}
	for _, script := range scripts {
		info, err := os.Stat(script)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s is not executable", script)
		}
		if output, err := exec.Command("sh", "-n", script).CombinedOutput(); err != nil {
			t.Errorf("sh -n %s: %v\n%s", script, err, output)
		}
	}

	builder := readText(t, scripts[0])
	for _, artifact := range []string{
		"audit config-manager console gateway runner ssh-broker preflight",
		`"$staging/bin/safeterm-$entry"`,
		"SHA256SUMS",
	} {
		if !strings.Contains(builder, artifact) {
			t.Errorf("bundle builder does not include %s", artifact)
		}
	}
}

// TEST-PACKAGE-001: REQ-017 AC-1 through AC-10.
func TestOfflinePackageIsCompleteAndPreservesUpgradeState(t *testing.T) {
	repository := repositoryRoot(t)
	creator := readText(t, filepath.Join(repository, "deploy", "package", "create-package.sh"))
	installer := readText(t, filepath.Join(repository, "deploy", "package", "install-package.sh"))
	for _, marker := range []string{
		"CGO_ENABLED=0 GOOS=linux", "amd64|arm64", "catalog-prepare catalog-sign",
		"PACKAGE-SHA256SUMS", "payload/base-install.sh",
	} {
		if !strings.Contains(creator, marker) {
			t.Errorf("offline package builder is missing %q", marker)
		}
	}
	for _, marker := range []string{
		"sha256sum -c PACKAGE-SHA256SUMS", "--prepare-catalog", "--signed-catalog",
		"--local-signing", "--ssh-public-key", "--single-operator", "--dry-run",
		"/etc/safeterm/ssh/ssh_host_ed25519_key", "/etc/safeterm/ssh/users.json",
		"/etc/safeterm/console/users.json", "/etc/safeterm/config-manager/resources.json",
		"preserved-managed", "safeterm-preflight", "--no-start",
	} {
		if !strings.Contains(installer, marker) {
			t.Errorf("offline target installer is missing %q", marker)
		}
	}
	checksum := strings.Index(installer, "sha256sum -c PACKAGE-SHA256SUMS")
	rootCheck := strings.Index(installer, `"$(id -u)" -eq 0`)
	if checksum < 0 || rootCheck < 0 || checksum > rootCheck {
		t.Fatal("package checksum verification must precede privileged installation")
	}
	for _, forbidden := range []string{"git clone", "go build", "apt install", "curl ", "wget "} {
		if strings.Contains(installer, forbidden) {
			t.Errorf("offline target installer contains network/build dependency %q", forbidden)
		}
	}
}

// TEST-DEPLOY-004: REQ-015 AC-4 through AC-7.
func TestInstallerVerifiesBeforeMutationAndStartsDependenciesInOrder(t *testing.T) {
	repository := repositoryRoot(t)
	installer := readText(t, filepath.Join(repository, "deploy", "scripts", "install.sh"))
	preflight := strings.Index(installer, "safeterm-preflight\" --bundle")
	firstMutation := strings.Index(installer, "for name in $groups")
	if preflight < 0 || firstMutation < 0 || preflight > firstMutation {
		t.Fatal("installer must run bundle preflight before account or host mutation")
	}
	if !strings.Contains(installer, "--dry-run") || !strings.Contains(installer, "sha256sum -c") || !strings.Contains(installer, "safeterm-preflight --installed") {
		t.Fatal("installer is missing dry-run, checksum, or installed-preflight enforcement")
	}
	for _, marker := range []string{"safeterm-console", "useradd --create-home", "usermod --shell", "/etc/safeterm/console/users.json"} {
		if !strings.Contains(installer, marker) {
			t.Errorf("installer is missing console-login marker %q", marker)
		}
	}

	order := []string{
		"safeterm-audit.service",
		"safeterm-runner-diagnostics.service",
		"safeterm-gateway.service",
		"safeterm-config-manager-application.service",
		"safeterm-ssh-broker.service",
	}
	position := -1
	for _, unit := range order {
		next := strings.Index(installer[position+1:], unit)
		if next < 0 {
			t.Fatalf("installer does not start %s in the required dependency sequence", unit)
		}
		position += next + 1
	}
}

// TEST-E2E-005..006: REQ-015 AC-10 through AC-12.
func TestOperatorDemoExercisesPublicWorkflowAndReportsPass(t *testing.T) {
	repository := repositoryRoot(t)
	demo := readText(t, filepath.Join(repository, "deploy", "demo", "run-demo.sh"))
	for _, required := range []string{
		"ssh-keygen", "ssh-keyscan", "StrictHostKeyChecking=yes",
		"whoami", "list", "run system_info", "config list",
		"result: success", "safeterm-preflight --installed",
		"SafeTerm end-to-end demo: PASS",
	} {
		if !strings.Contains(demo, required) {
			t.Errorf("operator demo is missing %q", required)
		}
	}
	if !strings.Contains(readText(t, filepath.Join(repository, "docs", "deployment.md")), "Fresh-host scope") {
		t.Fatal("deployment guide must state the fresh-host scope")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
