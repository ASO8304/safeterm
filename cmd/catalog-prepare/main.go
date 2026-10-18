package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"safeterm/internal/config"
	"safeterm/internal/integrity"

	"safeterm/internal/deployment"
)

func main() {
	source := flag.String("source", "./internal/defaults/catalog", "source catalog directory")
	output := flag.String("output", "", "new unsigned deployment catalog directory")
	keep := flag.Bool("keep-hashes", false, "preserve approved executable hashes")
	compatible := flag.String("compatible-with", "", "require the same provisioned profile boundaries as this catalog")
	verify := flag.Bool("verify-source", false, "require a valid existing source signature")
	flag.Parse()
	if *verify {
		cfg, err := config.LoadConfig(*source)
		if err == nil {
			err = integrity.VerifyCatalogFiles(cfg, filepath.Join(*source, "trust/catalog-ed25519.pub"), filepath.Join(*source, "catalog.sig"))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "source catalog is not verified; direct edits are unsupported; rebuild from trusted packaged built-ins:", err)
			os.Exit(1)
		}
	}
	if *compatible != "" {
		if err := deployment.CheckProfileCompatibility(*source, *compatible); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := deployment.PrepareConfigWithHashes(*source, *output, !*keep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Prepared unsigned deployment catalog at %s\n", *output)
}
