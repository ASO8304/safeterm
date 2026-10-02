package configmanager

import (
	"strings"
	"testing"
)

func TestParseRegistryStrictOfflineValidation(t *testing.T) {
	valid := `{
  "version":"v1",
  "resources":[{
    "id":"status",
    "description":"status",
    "profile":"application",
    "path":"/etc/safeterm-managed/application/status.json",
    "format":"json",
    "access":"read-only",
    "fields":[{"section":"root","key":"enabled","type":"boolean","required":true,"secret":false,"read_only":true}],
    "backup":{"enabled":false,"keep":0},
    "permissions":{"read_roles":["viewer"],"propose_roles":[],"apply_roles":[],"activate_roles":[],"rollback_roles":[]}
  }]
}`
	registry, err := ParseRegistry([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version != "v1" || len(registry.Resources) != 1 {
		t.Fatalf("registry=%+v", registry)
	}

	for name, input := range map[string]string{
		"unknown field": strings.Replace(valid, `"version":"v1"`, `"version":"v1","unexpected":true`, 1),
		"trailing data": valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRegistry([]byte(input)); err == nil {
				t.Fatal("invalid offline registry was accepted")
			}
		})
	}
	if _, err := ParseRegistry(make([]byte, maxRegistryBytes+1)); err == nil {
		t.Fatal("oversized offline registry was accepted")
	}
}
