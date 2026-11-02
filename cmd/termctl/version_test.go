package main

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionWorksWithoutConfigurationAndRejectsExtraArguments(t *testing.T) {
	for _, command := range []string{"version", "--version"} {
		var out bytes.Buffer
		if err := run([]string{"--config-dir", "/nonexistent/safeterm-version-test", command}, &out, &out); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"SafeTerm ", "Commit:", "Source:", "Go:", "Platform:", "Executable:"} {
			if !strings.Contains(out.String(), field) {
				t.Fatal("version metadata missing", field, out.String())
			}
		}
		if err := run([]string{command, "extra"}, &out, &out); err == nil {
			t.Fatal("extra version arguments accepted")
		}
	}
}

func TestVersionReportsStampedReleaseAndModifiedSourceHonestly(t *testing.T) {
	for _, modified := range []string{"true", "false"} {
		var out bytes.Buffer
		info := &debug.BuildInfo{GoVersion: "go1.26.10", Main: debug.Module{Version: "v1.2.3"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789abcdef"}, {Key: "vcs.modified", Value: modified}}}
		writeVersion(&out, info, "/usr/local/bin/termctl")
		state := "clean"
		if modified == "true" {
			state = "modified"
		}
		for _, field := range []string{"SafeTerm v1.2.3", "Commit: 0123456789abcdef", "Source: " + state, "Go: go1.26.10", "Executable: /usr/local/bin/termctl"} {
			if !strings.Contains(out.String(), field) {
				t.Fatal("incorrect build metadata", out.String())
			}
		}
	}
	var out bytes.Buffer
	writeVersion(&out, nil, "unknown")
	if !strings.Contains(out.String(), "SafeTerm development") || !strings.Contains(out.String(), "Source: unknown") {
		t.Fatal("unstamped build claimed a release", out.String())
	}
}
