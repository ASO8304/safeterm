package security_test

import (
	"context"
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
	cfg, err := config.LoadConfig("../../configs")
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
			if value == "os/exec" && !strings.HasPrefix(filepath.ToSlash(relative), "internal/execution/") {
				t.Errorf("production os/exec import outside Runner: %s", relative)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
					if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "exec" && (selector.Sel.Name == "Command" || selector.Sel.Name == "CommandContext") && filepath.ToSlash(relative) != "internal/execution/runner.go" {
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
		for _, required := range []string{"NoNewPrivileges=", "CapabilityBoundingSet=", "MemoryMax=", "TasksMax="} {
			if !strings.Contains(unit, required) {
				t.Errorf("%s missing %s", entry.Name(), required)
			}
		}
		if strings.Contains(unit, "CAP_SYS_ADMIN") || (strings.Contains(unit, "AmbientCapabilities=CAP_") && entry.Name() != "safeterm-runner-network-observer.service") {
			t.Errorf("%s grants broad ambient privilege", entry.Name())
		}
	}
}

// TEST-SECURITY-004: REQ-014 AC-3 and AC-7.
func TestSignedCatalogContainsNoShellOrCallerSelectedExecutable(t *testing.T) {
	cfg, err := config.LoadConfig("../../configs")
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
