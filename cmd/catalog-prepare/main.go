package main

import (
	"flag"
	"fmt"
	"os"

	"safeterm/internal/deployment"
)

func main() {
	source := flag.String("source", "./configs", "source catalog directory")
	output := flag.String("output", "", "new unsigned deployment catalog directory")
	flag.Parse()
	if err := deployment.PrepareConfig(*source, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Prepared unsigned deployment catalog at %s\n", *output)
}
