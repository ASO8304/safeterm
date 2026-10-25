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
func TestCombinedDefinitionKeepsServiceActionsSeparateFromSaving(t *testing.T) {
	item := parseTest(t, configOnly)
	service := parseTest(t, serviceOnly)
	item.Definition.Service = service.Definition.Service
	item.Definition.Config.OnChange = "restart"
	base, _ := compilerFixture(t)
	if _, _, _, err := Compile(base, []Named{item}); err == nil {
		t.Fatal("automatic service activation accepted")
	}
	item.Definition.Config.OnChange = ""
	_, registry, _, err := Compile(base, []Named{item})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range registry.Resources {
		if r.ID == "options" {
			if r.ActivationOperation != "" || r.RollbackOnFailure {
				t.Fatal("saving config must not activate a service")
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

func TestCompileNetworkFieldTypes(t *testing.T) {
	base, _ := compilerFixture(t)
	item := parseTest(t, `{"id":"jungle","description":"Jungle network settings","config":{"path":"/etc/jungle/settings.ini","format":"ini","permissions":{"read":["config-viewer"],"edit":["config-operator"]},"fields":[{"section":"Dog","key":"Address","type":"ipv4"},{"section":"Cat","key":"Address","type":"ipv6"},{"section":"Mouse","key":"Address","type":"ip"},{"section":"Network","key":"Subnet","type":"cidr"}]}}`)
	_, registry, _, err := Compile(base, []Named{item})
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Resources) != 1 {
		t.Fatalf("resources=%v", registry.Resources)
	}
	fields := registry.Resources[0].Fields
	if len(fields) != 4 {
		t.Fatalf("fields=%v", fields)
	}
	for i, typ := range []configmanager.FieldType{configmanager.FieldIPv4, configmanager.FieldIPv6, configmanager.FieldIP, configmanager.FieldCIDR} {
		if fields[i].Type != typ {
			t.Fatalf("field %d type=%s", i, fields[i].Type)
		}
	}
}

func TestDefinitionJSONErrorsHaveFileLineAndColumnWithoutValues(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{"{\n  ]\n}", "line 2, column 3"},
		{"{\r\n  ]\r\n}", "line 2, column 3"},
		{"{\n  \"id\":", "line 2, column 8"},
		{"{\n  \"id\": 42\n}", "line 2, column 10"},
		{"{\"id\":\"مثال\", ]}", "line 1, column 15"},
	} {
		_, err := Parse("resources.d/ks-nsp.json", []byte(tc.data))
		if err == nil || !strings.Contains(err.Error(), "resources.d/ks-nsp.json") || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: %v; want %s", tc.data, err, tc.want)
		}
	}
	const secret = "do-not-print-this-value"
	_, err := Parse("resource.json", []byte(`{"id":"`+secret+`", ]}`))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks document: %v", err)
	}
}

func TestCompileLogicalContextsAndNames(t *testing.T) {
	base, _ := compilerFixture(t)
	const source = `{"id":"shared-options","description":"Shared","config":{"path":"/etc/product/options.ini","format":"ini","permissions":{"read":["config-viewer"],"edit":["config-operator"]},"contexts":[{"path":["tunnel"],"fields":[{"name":"Value_1","section":"Tunnel","key":"TCPMSS","type":"integer","min":1,"max":65535},{"name":"Value_2","section":"Cat","key":"Port_2","type":"port"}]},{"path":["application","cat"],"fields":[{"name":"enabled","section":"Cat","key":"Enabled","type":"boolean"}]}]}}`
	item := parseTest(t, source)
	_, reg, _, err := Compile(base, []Named{item})
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Resources) != 1 || len(reg.Resources[0].Contexts) != 2 || len(reg.Resources[0].Fields) != 3 {
		t.Fatalf("wrong compiled registry %+v", reg)
	}
	if reg.Resources[0].Fields[0].Key != "TCPMSS" || reg.Resources[0].Contexts[0].Fields[0].Name != "Value_1" {
		t.Fatal("alias replaced physical key")
	}
	for _, broken := range []string{
		strings.Replace(source, `"name":"Value_1"`, `"name":"bad name"`, 1),
		strings.Replace(source, `"name":"Value_2"`, `"name":"Value_1"`, 1),
		strings.Replace(source, `["application","cat"]`, `["tunnel","cat"]`, 1),
		strings.Replace(source, `"contexts":`, `"fields":[{"section":"x","key":"Y","type":"string"}],"contexts":`, 1),
	} {
		item := parseTest(t, broken)
		if _, _, _, err := Compile(base, []Named{item}); err == nil {
			t.Fatal("accepted invalid logical definition")
		}
	}
}
