package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"safeterm/internal/deployment"
)

func main() {
	root := flag.String("bundle", "", "deployment bundle root")
	installed := flag.Bool("installed", false, "check the installed system paths and service accounts")
	jsonOutput := flag.Bool("json", false, "print the successful report as JSON")
	managedPlan := flag.Bool("managed-plan", false, "print validated managed-file install rows")
	flag.Parse()
	if *installed {
		*root = "/"
	}
	report, err := deployment.Check(deployment.CheckOptions{Root: *root, Installed: *installed})
	if err != nil {
		fmt.Fprintln(os.Stderr, "SafeTerm preflight: FAIL:", err)
		os.Exit(1)
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
	if *jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return
	}
	fmt.Printf("SafeTerm preflight: PASS (catalog=%s profiles=%d operations=%d executables=%d resources=%d units=%d)\n",
		report.CatalogVersion, report.Profiles, report.Operations, report.Executables, report.Resources, report.Units)
}
