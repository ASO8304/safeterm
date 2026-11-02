package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
)

func versionCommand(args []string, out io.Writer) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: termctl version")
	}
	info, _ := debug.ReadBuildInfo()
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	writeVersion(out, info, executable)
	return nil
}

// Go stamps the checked-out release tag and VCS revision into package builds.
// Read that binary's metadata without loading configuration or calling services.
func writeVersion(out io.Writer, info *debug.BuildInfo, executable string) {
	release, revision, source, compiler := "development", "unknown", "unknown", runtime.Version()
	if info != nil {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			release = info.Main.Version
		}
		if info.GoVersion != "" {
			compiler = info.GoVersion
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					source = "modified"
				} else if setting.Value == "false" {
					source = "clean"
				}
			}
		}
	}
	fmt.Fprintf(out, "SafeTerm %s\nCommit: %s\nSource: %s\nGo: %s\nPlatform: %s/%s\nExecutable: %s\n", release, revision, source, compiler, runtime.GOOS, runtime.GOARCH, executable)
}
