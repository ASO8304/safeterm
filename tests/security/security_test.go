package security_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"safeterm/internal/agent"
	"safeterm/internal/config"
	"safeterm/internal/contract"
	"safeterm/internal/policy"
	"safeterm/internal/translation"
	"safeterm/internal/validation"
)

type executionProbe struct{ calls int }

func (p *executionProbe) Execute(context.Context, string, []string) (string, string, int, error) {
	p.calls++
	return "", "", 0, nil
}

func securityCore(t *testing.T) (*agent.Core, *executionProbe) {
	t.Helper()
	cfg, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	translator, err := translation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	probe := &executionProbe{}
	return agent.NewCore(validation.New(cfg), policy.New(cfg), translator, probe), probe
}

// TEST-SECURITY-001: REQ-014 AC-2.
func TestInjectionCorpusNeverReachesExecution(t *testing.T) {
	core, probe := securityCore(t)
	payloads := []string{"127.0.0.1;id", "127.0.0.1 && id", "127.0.0.1|id", "`id`", "$(id)", "127.0.0.1 > /tmp/x", "' OR '1'='1", "\";id", "127.0.0.1\nwhoami", "127.0.0.1\rwhoami", "${IFS}id", "{id}", strings.Repeat("1", 8192)}
	for _, payload := range payloads {
		request, _ := contract.NewExecutionRequest("trace", map[string]string{"port": "53", "host": payload}, contract.Metadata{BrokerType: "ssh", UserID: "attacker", ClaimedRoles: []string{"netops"}})
		response := core.HandleRequest(context.Background(), request)
		if response.Result != contract.ResultRejected || response.FailureStage != string(agent.StageValidation) {
			t.Errorf("payload %q result=%s stage=%s", payload, response.Result, response.FailureStage)
		}
	}
	for _, port := range []string{"1;id", "1|id", "$(id)", "-1", "0", "65536", "999999999999999999999"} {
		request, _ := contract.NewExecutionRequest("trace", map[string]string{"port": port, "host": "127.0.0.1"}, contract.Metadata{BrokerType: "ssh", UserID: "attacker", ClaimedRoles: []string{"netops"}})
		response := core.HandleRequest(context.Background(), request)
		if response.Result != contract.ResultRejected || response.FailureStage != string(agent.StageValidation) {
			t.Errorf("port payload %q accepted", port)
		}
	}
	for _, command := range []string{"/bin/sh", "sh", "sudo", "systemctl", "../../bin/id", "trace;id"} {
		request, _ := contract.NewExecutionRequest(command, map[string]string{}, contract.Metadata{BrokerType: "ssh", UserID: "attacker", ClaimedRoles: []string{"netops"}})
		response := core.HandleRequest(context.Background(), request)
		if response.Result != contract.ResultRejected || response.FailureStage != string(agent.StageValidation) {
			t.Errorf("raw command %q accepted", command)
		}
	}
	if probe.calls != 0 {
		t.Fatalf("executor reached %d times", probe.calls)
	}
}

// TEST-SECURITY-002: REQ-014 AC-3.
func TestProductionSourceCannotIntroduceAnotherProcessOrShellBoundary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			if relative == ".git" || relative == "evidence" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			value, _ := strconv.Unquote(spec.Path.Value)
			trustedProcessBoundary := strings.HasPrefix(filepath.ToSlash(relative), "internal/execution/") || filepath.ToSlash(relative) == "cmd/termctl/main.go"
			if value == "os/exec" && !trustedProcessBoundary {
				t.Errorf("production os/exec import outside Runner: %s", relative)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
					if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "exec" && (selector.Sel.Name == "Command" || selector.Sel.Name == "CommandContext") && filepath.ToSlash(relative) != "internal/execution/runner.go" && filepath.ToSlash(relative) != "cmd/termctl/main.go" {
						t.Errorf("process creation outside bounded Runner: %s", relative)
					}
				}
			}
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, _ := strconv.Unquote(literal.Value)
			lower := strings.ToLower(value)
			for _, shell := range []string{"/bin/sh", "sh -c", "bash -c", "powershell.exe", "cmd.exe /c"} {
				if strings.Contains(lower, shell) && filepath.ToSlash(relative) != "internal/integrity/executor.go" {
					t.Errorf("shell literal %q in production file %s", shell, relative)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TEST-SECURITY-003: REQ-014 AC-8.
func TestDeployedServicesRetainExplicitResourceAndPrivilegeBounds(t *testing.T) {
	unitDir := filepath.Join("..", "..", "deploy", "systemd")
	entries, err := os.ReadDir(unitDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".service") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(unitDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		unit := string(data)
		noNewPrivileges, ok := systemdSetting(unit, "NoNewPrivileges")
		privilegedController := entry.Name() == "safeterm-runner-service-manager.service"
		if privilegedController {
			if !ok || noNewPrivileges != "no" || !strings.Contains(unit, "RestrictSUIDSGID=no") {
				t.Errorf("%s must permit only the sudoers-confined typed controller", entry.Name())
			}
		} else if !ok || (noNewPrivileges != "yes" && noNewPrivileges != "true") {
			t.Errorf("%s must enable NoNewPrivileges", entry.Name())
		}
		memoryMax, ok := systemdSetting(unit, "MemoryMax")
		if !ok || !finiteSystemdSize(memoryMax) {
			t.Errorf("%s has no finite positive MemoryMax", entry.Name())
		}
		tasksMax, ok := systemdSetting(unit, "TasksMax")
		parsedTasks, parseErr := strconv.ParseUint(tasksMax, 10, 32)
		if !ok || parseErr != nil || parsedTasks == 0 || parsedTasks > 1024 {
			t.Errorf("%s has invalid TasksMax %q", entry.Name(), tasksMax)
		}
		bounding, hasBounding := systemdSetting(unit, "CapabilityBoundingSet")
		ambient, hasAmbient := systemdSetting(unit, "AmbientCapabilities")
		if !hasBounding || !hasAmbient {
			t.Errorf("%s must explicitly define capability sets", entry.Name())
		}
		expectedCapabilities := ""
		expectedBounding := ""
		if entry.Name() == "safeterm-runner-network-observer.service" {
			expectedCapabilities = "CAP_NET_RAW"
			expectedBounding = expectedCapabilities
		}
		if entry.Name() == "safeterm-config-writer.service" {
			expectedBounding = "CAP_CHOWN CAP_FOWNER CAP_DAC_READ_SEARCH"
		}
		if privilegedController {
			expectedBounding = "CAP_SETUID CAP_SETGID CAP_AUDIT_WRITE"
			families, _ := systemdSetting(unit, "RestrictAddressFamilies")
			if families != "AF_UNIX AF_NETLINK" {
				t.Errorf("%s must permit sudo's local audit transport", entry.Name())
			}
		}
		if bounding != expectedBounding || ambient != expectedCapabilities {
			t.Errorf("%s capabilities bounding=%q ambient=%q, want bounding=%q ambient=%q", entry.Name(), bounding, ambient, expectedBounding, expectedCapabilities)
		}
	}
}

func systemdSetting(unit, key string) (string, bool) {
	var value string
	found := false
	for _, line := range strings.Split(unit, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			value = strings.TrimSpace(strings.TrimPrefix(line, key+"="))
			found = true
		}
	}
	return value, found
}

func finiteSystemdSize(value string) bool {
	if value == "" || strings.EqualFold(value, "infinity") {
		return false
	}
	digits := strings.TrimRight(value, "KMGTPE")
	if digits == "" || len(digits) < len(value)-1 {
		return false
	}
	parsed, err := strconv.ParseUint(digits, 10, 64)
	return err == nil && parsed > 0
}

func TestSystemdBoundValidationRejectsUnsafeEffectiveValues(t *testing.T) {
	for _, value := range []string{"", "0", "infinity", "Infinity", "unbounded", "128MB"} {
		if finiteSystemdSize(value) {
			t.Errorf("unsafe MemoryMax value %q accepted", value)
		}
	}
	for _, value := range []string{"1", "128M", "2G"} {
		if !finiteSystemdSize(value) {
			t.Errorf("finite MemoryMax value %q rejected", value)
		}
	}
	if value, ok := systemdSetting("MemoryMax=128M\nMemoryMax=infinity\n", "MemoryMax"); !ok || value != "infinity" {
		t.Fatalf("effective duplicate setting=%q found=%v", value, ok)
	}
}

// TEST-SECURITY-004: REQ-014 AC-3 and AC-7.
func TestSignedCatalogContainsNoShellOrCallerSelectedExecutable(t *testing.T) {
	cfg, err := config.LoadConfig("../../internal/defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	executables := map[string]config.ExecutableDefinition{}
	for _, executable := range cfg.Executables {
		executables[executable.ID] = executable
		lower := strings.ToLower(executable.Path)
		if strings.HasSuffix(lower, "/sh") || strings.Contains(lower, "bash") || strings.Contains(lower, "powershell") || strings.HasSuffix(lower, "cmd.exe") {
			t.Errorf("shell registered as executable: %s", executable.Path)
		}
	}
	for _, operation := range cfg.Operations {
		if _, ok := executables[operation.ExecutableRef]; operation.ExecutorKind == config.ExecutorProcess && !ok {
			t.Errorf("operation %s lacks registered executable", operation.Name)
		}
		for _, token := range operation.ArgsTemplate {
			if strings.Contains(token, "{") && !(strings.HasPrefix(token, "{") && strings.HasSuffix(token, "}") && strings.Count(token, "{") == 1 && strings.Count(token, "}") == 1) {
				t.Errorf("partial placeholder in %s: %q", operation.Name, token)
			}
		}
	}
}

// TEST-SECURITY-018: REQ-014 AC-1, AC-13, and AC-14.
func TestM14SecurityArtifactsStayComplete(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	requiredContent := map[string][]string{
		"docs/security/threat-model.md": {
			"## Scope and assets", "## Trust boundaries", "## Invariants and verification",
			"## Exclusions and residual risk", "| Threat | Required control | Verification |",
		},
		".github/workflows/security.yml": {
			"permissions:\n  contents: read", "make security", "golang/govulncheck-action",
			"go mod verify", "go list -m all", "github/codeql-action/analyze", "Enforce zero CodeQL findings",
		},
	}
	for relative, required := range requiredContent {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		content := string(data)
		for _, marker := range required {
			if !strings.Contains(content, marker) {
				t.Errorf("%s missing required marker %q", relative, marker)
			}
		}
	}

	for _, name := range []string{"make-verify.log", "security.log", "fuzz.log", "govulncheck-ci.log"} {
		info, err := os.Stat(filepath.Join(root, "evidence", "M14", name))
		if err != nil || info.Size() == 0 {
			t.Errorf("M14 evidence %s missing or empty: %v", name, err)
		}
	}

	matrix, err := os.ReadFile(filepath.Join(root, "docs", "testing", "test-matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	matrixText := string(matrix)
	for ac := 1; ac <= 14; ac++ {
		commaMarker := fmt.Sprintf("AC-%d,", ac)
		closingMarker := fmt.Sprintf("AC-%d)", ac)
		mapped := false
		for _, line := range strings.Split(matrixText, "\n") {
			if strings.Contains(line, "REQ-014 (") && (strings.Contains(line, commaMarker) || strings.Contains(line, closingMarker)) {
				mapped = true
				break
			}
		}
		if !mapped {
			t.Errorf("test matrix does not map REQ-014 AC-%d", ac)
		}
	}
}
