package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"safeterm/internal/configmanager"
	"safeterm/internal/layout"

	"safeterm/internal/deployment"
)

func main() {
	priorIdentityCatalog := flag.String("prior-identity-catalog", "", "verified previous catalog for credential validation during package staging only")
	root := flag.String("bundle", "", "deployment bundle root")
	installed := flag.Bool("installed", false, "check the installed system paths and service accounts")
	jsonOutput := flag.Bool("json", false, "print the successful report as JSON")
	managedPlan := flag.Bool("managed-plan", false, "print validated managed-file install rows")
	sudoersPlan := flag.Bool("sudoers-plan", false, "print exact sudoers rules for typed systemd operations")
	stageManaged := flag.Bool("stage-managed-upgrade", false, "stage existing registered files into a private installer bundle")
	installConfig := flag.Bool("install-configuration", false, "install validated configuration preserving host files")
	writerPlan := flag.Bool("writer-plan", false, "print privileged writer sandbox drop-in")
	checkAdminUpgrade := flag.Bool("check-administration-upgrade", false, "check administrator layout migration without changing the host")
	flag.Parse()
	if *checkAdminUpgrade {
		if os.Geteuid() != 0 || *root != "" || *installed {
			fmt.Fprintln(os.Stderr, "administrator upgrade check requires root and no bundle/installed mode")
			os.Exit(2)
		}
		if err := layout.CheckAdministrationMigration(layout.Admin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *stageManaged || *installConfig {
		if os.Geteuid() != 0 || *root == "" || *installed || (*stageManaged && *installConfig) {
			fmt.Fprintln(os.Stderr, "installer helper requires root and one --bundle mode")
			os.Exit(2)
		}
	}
	if *stageManaged {
		if err := deployment.StageManagedUpgrade(*root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *installed {
		*root = "/"
	}
	if *priorIdentityCatalog != "" && (os.Geteuid() != 0 || *installed) {
		fmt.Fprintln(os.Stderr, "prior identity roles require privileged package staging")
		os.Exit(2)
	}
	report, err := deployment.Check(deployment.CheckOptions{Root: *root, Installed: *installed, PriorIdentityCatalog: *priorIdentityCatalog})
	if err != nil {
		fmt.Fprintln(os.Stderr, "SafeTerm preflight: FAIL:", err)
		os.Exit(1)
	}
	if *installConfig {
		if err := deployment.InstallAdministration(*root+"/etc/safeterm", layout.Admin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := deployment.InstallGenerated(*root+"/etc/safeterm", layout.Generated); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := deployment.InstallGenerated(*root+"/etc/safeterm", layout.Defaults); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *writerPlan {
		data, err := os.ReadFile(filepath.Join(*root, "etc/safeterm/resources.json"))
		if err == nil {
			var registry *configmanager.Registry
			registry, err = configmanager.ParseRegistry(data)
			if err == nil {
				data, err = configmanager.WriterUnitDropIn(registry, true)
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Stdout.Write(data)
		return
	}
	if *managedPlan {
		if *installed {
			fmt.Fprintln(os.Stderr, "SafeTerm preflight: --managed-plan requires --bundle")
			os.Exit(2)
		}
		files, err := deployment.ManagedFiles(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "SafeTerm preflight: FAIL:", err)
			os.Exit(1)
		}
		for _, file := range files {
			fmt.Printf("%s\t%s\t%s\n", file.Source, file.Target, file.User)
		}
		return
	}
	if *sudoersPlan {
		if *installed {
			fmt.Fprintln(os.Stderr, "SafeTerm preflight: --sudoers-plan requires --bundle")
			os.Exit(2)
		}
		lines, err := deployment.SystemdSudoers(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "SafeTerm preflight: FAIL:", err)
			os.Exit(1)
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		return
	}
	if *jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return
	}
	fmt.Printf("SafeTerm preflight: PASS (catalog=%s profiles=%d operations=%d executables=%d resources=%d units=%d)\n",
		report.CatalogVersion, report.Profiles, report.Operations, report.Executables, report.Resources, report.Units)
}
