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
func TestConfigManagerUnitsCanPreserveManagedGroupWithoutPrivileges(t *testing.T) {
	for _, profile := range []string{"application", "database", "network"} {
		unit := readText(t, filepath.Join(repositoryRoot(t), "deploy", "systemd", "safeterm-config-manager-"+profile+".service"))
		settings := map[string]string{}
		for _, line := range strings.Split(unit, "\n") {
			if key, value, ok := strings.Cut(line, "="); ok {
				settings[key] = value
			}
		}
		owner := "safeterm-config-" + profile
		if settings["User"] != owner {
			t.Errorf("%s: incorrect managed-file owner", profile)
		}
		groups := " " + settings["SupplementaryGroups"] + " "
		if settings["Group"] != owner && !strings.Contains(groups, " "+owner+" ") {
			t.Errorf("%s: cannot preserve managed files' private group", profile)
		}
		if settings["CapabilityBoundingSet"] != "" || settings["AmbientCapabilities"] != "" {
			t.Errorf("%s: group preservation must not require capabilities", profile)
		}
	}
}

func TestConsoleIdentityUnitHasIsolatedReadOnlyCredentials(t *testing.T) {
	unit := readText(t, filepath.Join(repositoryRoot(t), "deploy", "systemd", "safeterm-identity.service"))
	settings := map[string]string{}
	for _, line := range strings.Split(unit, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			settings[key] = value
		}
	}
	if settings["User"] != "safeterm-identity" || settings["NoNewPrivileges"] != "yes" || settings["ProtectSystem"] != "strict" || settings["LimitCORE"] != "0" || settings["RestrictAddressFamilies"] != "AF_UNIX" {
		t.Fatal("Identity service must isolate credential handling")
	}
	if settings["CapabilityBoundingSet"] != "" || settings["AmbientCapabilities"] != "" || settings["ReadWritePaths"] != "" {
		t.Fatal("Identity must not execute privileged operations or modify credentials")
	}
}

func TestDeploymentScriptsAreExecutableValidShellAndComplete(t *testing.T) {
	repository := repositoryRoot(t)
	scripts := []string{
		filepath.Join(repository, "deploy", "scripts", "build-bundle.sh"),
		filepath.Join(repository, "deploy", "scripts", "install.sh"),
		filepath.Join(repository, "deploy", "demo", "run-demo.sh"),
		filepath.Join(repository, "deploy", "package", "create-package.sh"),
		filepath.Join(repository, "deploy", "package", "install-package.sh"),
		filepath.Join(repository, "deploy", "package", "install-support.sh"),
		filepath.Join(repository, "deploy", "package", "rollback-package.sh"),
		filepath.Join(repository, "deploy", "ansible", "install.sh"),
		filepath.Join(repository, "deploy", "ansible", "rollback.sh"),
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
		"audit config-manager config-writer console gateway runner ssh-broker preflight",
		`"$staging/bin/safeterm-$entry"`,
		"SHA256SUMS",
	} {
		if !strings.Contains(builder, artifact) {
			t.Errorf("bundle builder does not include %s", artifact)
		}
	}
}

// TEST-ANSIBLE-001: REQ-020 AC-1 through AC-11.
func TestAnsibleInstallerOrchestratesVerifiedTransactionalPackage(t *testing.T) {
	repository := repositoryRoot(t)
	files := []string{
		"deploy/ansible/install.yml",
		"deploy/ansible/verify.yml",
		"deploy/ansible/rollback.yml",
		"deploy/ansible/group_vars/all.example.yml",
		"deploy/ansible/roles/safeterm/defaults/main.yml",
		"deploy/ansible/roles/safeterm/tasks/main.yml",
		"deploy/ansible/roles/safeterm/tasks/dependencies.yml",
		"deploy/ansible/roles/safeterm/tasks/package.yml",
		"deploy/ansible/roles/safeterm/tasks/configuration.yml",
		"deploy/ansible/roles/safeterm/handlers/main.yml",
	}
	var combined strings.Builder
	for _, relative := range files {
		combined.WriteString(readText(t, filepath.Join(repository, filepath.FromSlash(relative))))
		combined.WriteByte('\n')
	}
	content := combined.String()
	for _, marker := range []string{
		"SAFETERM_PYTHON_BOOTSTRAPPED",
		"safeterm_apt_mode in ['online', 'repository', 'preinstalled']",
		"ansible.builtin.apt_repository",
		"ansible.builtin.apt:",
		"checksum_algorithm: sha256",
		"Refuse a damaged package transfer",
		"--dry-run",
		"ansible-deployment",
		"safeterm-preflight --installed",
		"systemctl, is-active",
		"safeterm-install-rollback",
		"Restart SafeTerm control plane",
	} {
		if !strings.Contains(content, marker) {
			t.Errorf("Ansible deployment assets are missing %q", marker)
		}
	}

	packageTasks := readText(t, filepath.Join(repository, "deploy", "ansible", "roles", "safeterm", "tasks", "package.yml"))
	dryRun := strings.Index(packageTasks, "Dry-run the verified package installer before mutation")
	install := strings.Index(packageTasks, "Install or upgrade SafeTerm transactionally")
	if dryRun < 0 || install < 0 || dryRun > install {
		t.Fatal("Ansible must dry-run the package installer before host mutation")
	}
}

// TEST-PACKAGE-001: REQ-017 AC-1 through AC-10.
func TestOfflinePackageIsCompleteAndPreservesUpgradeState(t *testing.T) {
	repository := repositoryRoot(t)
	creator := readText(t, filepath.Join(repository, "deploy", "package", "create-package.sh"))
	installer := readText(t, filepath.Join(repository, "deploy", "package", "install-package.sh")) + readText(t, filepath.Join(repository, "deploy", "package", "install-support.sh"))
	rollback := readText(t, filepath.Join(repository, "deploy", "package", "rollback-package.sh"))
	for _, marker := range []string{
		"CGO_ENABLED=0 GOOS=linux", "amd64|arm64", "catalog-prepare catalog-sign",
		"PACKAGE-SHA256SUMS", "payload/base-install.sh", "payload/rollback.sh",
	} {
		if !strings.Contains(creator, marker) {
			t.Errorf("offline package builder is missing %q", marker)
		}
	}
	for _, marker := range []string{
		"sha256sum -c PACKAGE-SHA256SUMS", "--prepare-catalog", "--signed-catalog",
		"--local-signing", "--ssh-public-key", "--dry-run",
		"/etc/safeterm/ssh/ssh_host_ed25519_key", "/etc/safeterm/ssh/users.json",
		"/etc/safeterm/console/users.json", "$payload/resources.json",
		"--stage-managed-upgrade", "--keep-hashes", "/usr/share/safeterm/defaults", "safeterm-preflight", "--no-start",
		"safeterm-runner-log-reader.service", "/usr/bin/tail", "/usr/bin/sudo",
		"stop_units_if_present()", `systemctl cat "$unit"`,
	} {
		if !strings.Contains(installer, marker) {
			t.Errorf("offline target installer is missing %q", marker)
		}
	}
	if strings.Contains(installer, "systemctl stop safeterm-runner-log-reader.service") {
		t.Fatal("upgrade stops a newly introduced unit without checking that it is installed")
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
	for _, marker := range []string{
		"/var/backups/safeterm-installer", "snapshot.tar", "replace-paths",
		"service-state", "SNAPSHOT-SHA256SUMS", "/etc/passwd",
		"/usr/local/sbin/safeterm-install-rollback",
	} {
		if !strings.Contains(installer, marker) {
			t.Errorf("offline target installer is missing rollback marker %q", marker)
		}
	}
	for _, marker := range []string{
		"sha256sum -c SNAPSHOT-SHA256SUMS", "systemctl disable", "tar -C / -xpf",
		"systemctl daemon-reload", "systemctl enable", "systemctl start",
		"SafeTerm installation rollback: PASS",
	} {
		if !strings.Contains(rollback, marker) {
			t.Errorf("installation rollback is missing %q", marker)
		}
	}
	transaction := strings.LastIndex(installer, "\ncreate_install_transaction\n")
	localKeyInstall := strings.Index(installer, `install -d -o root -g root -m 0700 "$local_key_dir"`)
	if transaction < 0 || localKeyInstall < 0 || transaction > localKeyInstall {
		t.Fatal("rollback transaction must precede persistent local signing-key creation")
	}
}

func TestOperatorCatalogTemplatesAndPrivilegeAssets(t *testing.T) {
	repository := repositoryRoot(t)
	for _, path := range []string{
		"configs/resources.d/TEMPLATE.json", "configs/resources.d/README.md", "configs/resources.d/README.txt",
		"deploy/ssh/users.template.jsonc", "deploy/console/users.template.jsonc",
	} {
		if _, err := os.Stat(filepath.Join(repository, filepath.FromSlash(path))); err != nil {
			t.Errorf("missing administrator template %s: %v", path, err)
		}
	}
	creator := readText(t, filepath.Join(repository, "deploy", "package", "create-package.sh"))
	for _, marker := range []string{"payload/templates", "ssh-users.template.jsonc", "console-users.template.jsonc"} {
		if !strings.Contains(creator, marker) {
			t.Errorf("offline package omits administrator template %q", marker)
		}
	}
	installer := readText(t, filepath.Join(repository, "deploy", "scripts", "install.sh"))
	for _, marker := range []string{"--sudoers-plan", "visudo -cf", "/etc/sudoers.d/safeterm-service-manager", "safeterm-log-reader", "/var/log/safeterm-readable"} {
		if !strings.Contains(installer, marker) {
			t.Errorf("installer missing operator control %q", marker)
		}
	}
	shell := readText(t, filepath.Join(repository, "internal", "broker", "shell.go"))
	for _, marker := range []string{"execute ?", "show COMMAND", "config RESOURCE", "Ctrl+]"} {
		if !strings.Contains(shell, marker) {
			t.Errorf("guided shell help missing %q", marker)
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
		"whoami", "execute ?", "execute system", "config ?",
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
