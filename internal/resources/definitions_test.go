package resources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/config"
	"safeterm/internal/configmanager"
)

func compilerFixture(t *testing.T) (*config.Config, *configmanager.Registry) {
	t.Helper()
	cfg, err := config.LoadConfig(filepath.Join("..", "defaults", "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "runtime", "resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := configmanager.ParseRegistry(data)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, reg
}
func parseTest(t *testing.T, text string) Named {
	t.Helper()
	n, err := Parse("example.json", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

const configOnly = `{"id":"options","description":"Options","config":{"path":"/etc/product/options.ini","format":"ini","permissions":{"read":["config-viewer","config-operator"],"edit":["config-operator"],"rollback":["config-operator"]},"fields":[{"section":"Server","key":"Port","type":"integer","min":1,"max":65535}]}}`
const logsOnly = `{"id":"output","description":"Output","logs":[{"id":"main","path":"/var/log/product/output.log","roles":["log-viewer"]},{"id":"secret","path":"/var/log/product/private.log","roles":["config-operator"]}]}`
const serviceOnly = `{"id":"myapp","description":"My app","service":{"unit":"myapp.service","actions":{"status":{"roles":["service-viewer"]},"restart":{"roles":["service-operator"]}}}}`

func TestIndependentSectionsRebuildAndRemoveFromPackagedBuiltins(t *testing.T) {
	base, reg := compilerFixture(t)
	before, _ := json.Marshal(base)
	regBefore, _ := json.Marshal(reg)
	items := []Named{parseTest(t, configOnly), parseTest(t, logsOnly), parseTest(t, serviceOnly)}
	cfg, registry, _, err := Compile(base, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Operations) != len(base.Operations)+4 || len(registry.Resources) != 1 {
		t.Fatal("wrong generated counts")
	}
	r := registry.Resources[len(registry.Resources)-1]
	for _, item := range registry.Resources {
		if item.ID == "options" {
			r = item
		}
	}
	if !r.PrivilegedWrite || r.ActivationOperation != "" || r.Profile != "application" || r.Backup.Keep != 5 {
		t.Fatalf("config-only unexpectedly bound to service: %+v", r)
	}
	for _, op := range cfg.Operations {
		if op.Name == OperationID("output", "log", "main") {
			if len(op.Parameters) != 0 || strings.Join(op.ArgsTemplate, " ") != "-n 50 -F -- /var/log/product/output.log" {
				t.Fatal("log is not fixed")
			}
		}
	}
	for _, p := range cfg.Policies {
		for _, op := range p.Operations {
			if op == OperationID("output", "log", "secret") && p.Role != "config-operator" {
				t.Fatal("per-log role leaked")
			}
		}
	}
	after, _ := json.Marshal(base)
	regAfter, _ := json.Marshal(reg)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(regBefore, regAfter) {
		t.Fatal("compiler mutated inputs")
	}
	next, nextReg, nextManifest, err := Compile(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Operations) != len(base.Operations) || len(nextReg.Resources) != 0 || len(nextManifest.Operations) != 0 {
		t.Fatal("removal retained generated definitions or changed packaged built-ins")
	}
}
func TestStrictDefinitionsAndInvalidReferencesFailClosed(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"id":"ok","id":"bad","description":"duplicate","logs":[]}`, `{"id":"ok","description":"x","logs":[],"unknown":true}`, `{"id":"ok","description":"x"} {}`, `{"id":"bad_id","description":"x","service":{}}`} {
		if _, err := Parse("broken.json", []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	base, _ := compilerFixture(t)
	cases := []string{
		strings.Replace(serviceOnly, "restart", "shell", 1),
		strings.Replace(serviceOnly, "service-operator", "missing-role", 1),
		strings.Replace(serviceOnly, "myapp.service", "*.service", 1),
		strings.Replace(logsOnly, "/var/log/product/output.log", "/etc/safeterm/ssh/users.json", 1),
		strings.Replace(configOnly, "\"format\":\"ini\"", "\"format\":\"ini\",\"on_change\":\"restart\"", 1),
		strings.Replace(configOnly, "config-operator", "unknown-role", 1),
	}
	for _, raw := range cases {
		item := parseTest(t, raw)
		if _, _, _, err := Compile(base, []Named{item}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	item := parseTest(t, configOnly)
	if _, _, _, err := Compile(base, []Named{item, item}); err == nil {
		t.Fatal("duplicate id accepted")
	}

}
func TestCombinedDefinitionHasExplicitActivationAndFriendlyPaths(t *testing.T) {
	item := parseTest(t, configOnly)
	service := parseTest(t, serviceOnly)
	item.Definition.Service = service.Definition.Service
	item.Definition.Config.OnChange = "restart"
	base, _ := compilerFixture(t)
	_, registry, _, err := Compile(base, []Named{item})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range registry.Resources {
		if r.ID == "options" {
			if r.ActivationOperation != OperationID("options", "service", "restart") || !r.RollbackOnFailure {
				t.Fatal("activation missing")
			}
		}
	}
	for name, want := range map[string]string{OperationID("myapp", "service", "status"): "service status myapp", OperationID("output", "log", "main"): "logs output main"} {
		words, ok := OperationWords(name)
		if !ok || strings.Join(words, " ") != want {
			t.Fatalf("%s => %v", name, words)
		}
	}
}
func FuzzDefinitionsNeverPanic(f *testing.F) {
	f.Add(configOnly)
	f.Add(serviceOnly)
	f.Add(`{"config":{"fields":[[[[]]]]}}`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > MaxFileBytes {
			return
		}
		_, _ = Parse("fuzz.json", []byte(s))
	})
}
