package siteconfig

import (
	"encoding/json"
	"safeterm/internal/config"
	"safeterm/internal/resources"
	"strings"
	"testing"
)

func fixture(t *testing.T) *config.Config {
	t.Helper()
	c, e := config.LoadConfig("../defaults/catalog")
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func stringsPtr(s ...string) *[]string { return &s }
func TestNoOverridesExactlyPreserveDefaults(t *testing.T) {
	base := fixture(t)
	before, _ := json.Marshal(base)
	got, e := Prepare(base, Inputs{})
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(got)
	if string(before) != string(after) {
		t.Fatal("no-input compiler changed defaults")
	}
}
func TestCustomRoleOperationExecutableAndIsolatedProfileCompile(t *testing.T) {
	base := fixture(t)
	before, _ := json.Marshal(base)
	in := Inputs{
		Roles:      []Named[Role]{{File: "access/roles.d/a.json", Value: Role{Name: "site-reader", Description: "Read site"}}},
		Profiles:   []Named[Profile]{{File: "commands/profiles.d/a.json", Value: Profile{Name: "site-demo", Base: config.ProfileDiagnostics, MemoryLimitMiB: 96, TimeoutSeconds: 12}}},
		Operations: []Named[Operation]{{File: "commands/operations.d/a.json", Value: Operation{Operation: config.OperationDefinition{Name: "site_system", Description: "Site system", RunnerProfile: "site-demo", ExecutorKind: config.ExecutorProcess, ExecutableRef: "uname", ArgsTemplate: []string{"-s"}}, Roles: stringsPtr("site-reader")}}},
	}
	c, e := Prepare(base, in)
	if e != nil {
		t.Fatal(e)
	}
	c, reg, _, e := resources.Compile(c, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(reg.Resources) != 0 {
		t.Fatal("unexpected resources")
	}
	if e = Finish(c, in); e != nil {
		t.Fatal(e)
	}
	if e = ValidateCatalog(c); e != nil {
		t.Fatal(e)
	}
	p := c.Profiles[len(c.Profiles)-1]
	if p.RunAsUser != User("site-demo") || p.SocketPath != Socket("site-demo") || p.MemoryLimitMiB != 96 || p.TimeoutSeconds != 12 {
		t.Fatalf("unexpected boundary: %+v", p)
	}
	after, _ := json.Marshal(base)
	if string(before) != string(after) {
		t.Fatal("mutated verified defaults")
	}
	removed, e := Prepare(base, Inputs{})
	if e != nil {
		t.Fatal(e)
	}
	if len(removed.Profiles) != len(base.Profiles) || len(removed.Roles) != len(base.Roles) {
		t.Fatal("removed site definitions retained")
	}
}
func TestDefaultOverrideAndExactPolicyReplacement(t *testing.T) {
	base := fixture(t)
	op := base.Operations[0]
	op.Description = "custom description"
	in := Inputs{Operations: []Named[Operation]{{Value: Operation{Operation: op, Roles: stringsPtr("viewer")}}}, Policies: []Named[config.PolicyDefinition]{{Value: config.PolicyDefinition{Role: "viewer", Operations: []string{op.Name}}}}}
	c, e := Prepare(base, in)
	if e != nil {
		t.Fatal(e)
	}
	if e = Finish(c, in); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, v := range c.Operations {
		if v.Name == op.Name {
			count++
			if v.Description != op.Description {
				t.Fatal("override ignored")
			}
		}
	}
	if count != 1 {
		t.Fatal("override added duplicate")
	}
	for _, p := range c.Policies {
		if p.Role == "viewer" && strings.Join(p.Operations, ",") != op.Name {
			t.Fatal("policy retained inherited grants")
		}
	}
	in.Operations[0].Value.Disabled = true
	c, e = Prepare(base, in)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range c.Policies {
		for _, name := range p.Operations {
			if name == op.Name {
				t.Fatal("disabled operation retained grants")
			}
		}
	}
}
func TestProfileCeilingsAndStrictJSON(t *testing.T) {
	base := fixture(t)
	for _, p := range []Profile{
		{Name: "escape\nUser=root", Base: config.ProfileDiagnostics},
		{Name: "demo", Base: "missing"},
		{Name: "diagnostics", Base: config.ProfileNetworkObserver},
		{Name: "demo", Base: config.ProfileDiagnostics, Capabilities: stringsPtr("CAP_SYS_ADMIN")},
		{Name: "demo", Base: config.ProfileNetworkObserver, Capabilities: stringsPtr("CAP_NET_ADMIN")},
		{Name: "demo", Base: config.ProfileDiagnostics, MemoryLimitMiB: 1},
		{Name: "demo", Base: config.ProfileDiagnostics, TimeoutSeconds: 86401},
	} {
		if _, e := Prepare(base, Inputs{Profiles: []Named[Profile]{{Value: p}}}); e == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	for _, raw := range []string{`{"name":"x","name":"y","base":"diagnostics"}`, `{"name":"x","base":"diagnostics","run_as_user":"root"}`, `{"name":"x","base":"diagnostics","socket_path":"/tmp/attack"}`} {
		if _, e := Parse[Profile]("commands/profiles.d/test.json", []byte(raw)); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestGeneratedUnitCannotBroadenIsolation(t *testing.T) {
	for _, base := range []config.RunnerProfile{config.ProfileDiagnostics, config.ProfileNetworkObserver, config.ProfileLogReader, config.ProfileServiceManager} {
		c, e := Prepare(fixture(t), Inputs{Profiles: []Named[Profile]{{Value: Profile{Name: "demo", Base: base}}}})
		if e != nil {
			t.Fatal(e)
		}
		p := c.Profiles[len(c.Profiles)-1]
		unit, e := RunnerUnit(p)
		if e != nil {
			t.Fatal(e)
		}
		for _, required := range []string{"User=sft-site-demo", "ProtectSystem=strict", "ProtectHome=yes", "PrivateDevices=yes", "RuntimeDirectory=safeterm-site-demo"} {
			if !strings.Contains(unit, required) {
				t.Fatal("missing " + required)
			}
		}
		if base != config.ProfileServiceManager && !strings.Contains(unit, "NoNewPrivileges=yes") {
			t.Fatal("lost NNP")
		}
		if strings.Contains(unit, "CAP_SYS_ADMIN") || strings.Contains(unit, "ReadWritePaths=") {
			t.Fatal("broadened unit")
		}
	}
}
func TestInvalidReferencesAndDuplicatesFail(t *testing.T) {
	base := fixture(t)
	role := Named[Role]{Value: Role{Name: "new-role", Description: "Read"}}
	if _, e := Prepare(base, Inputs{Roles: []Named[Role]{role, role}}); e == nil {
		t.Fatal("duplicate roles")
	}
	c, e := Prepare(base, Inputs{})
	if e != nil {
		t.Fatal(e)
	}
	if e = Finish(c, Inputs{Policies: []Named[config.PolicyDefinition]{{Value: config.PolicyDefinition{Role: "missing", Operations: []string{"ping"}}}}}); e == nil {
		t.Fatal("unknown role allowed")
	}
}

func TestCustomExecutableApprovalIsExplicitAndNotRefreshed(t *testing.T) {
	base := fixture(t)
	executable := config.ExecutableDefinition{ID: "site_uptime", Path: "/usr/bin/uptime", SHA256: strings.Repeat("a", 64), AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics}}
	op := config.OperationDefinition{Name: "site_uptime", Description: "Site uptime", RunnerProfile: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess, ExecutableRef: executable.ID, ArgsTemplate: []string{"-p"}}
	input := Inputs{Executables: []Named[config.ExecutableDefinition]{{Value: executable}}, Operations: []Named[Operation]{{Value: Operation{Operation: op, Roles: stringsPtr("viewer")}}}}
	cfg, err := Prepare(base, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = Finish(cfg, input); err != nil {
		t.Fatal(err)
	}
	for _, e := range cfg.Executables {
		if e.ID == executable.ID && e.SHA256 != executable.SHA256 {
			t.Fatal("compiler refreshed an explicit approval")
		}
	}
	input.Executables[0].Value.SHA256 = "auto"
	cfg, err = Prepare(base, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = Finish(cfg, input); err == nil {
		t.Fatal("automatic approval accepted")
	}
}
