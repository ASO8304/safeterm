package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpAndCompletion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help"}, &out, &out); err != nil || !strings.Contains(out.String(), "termctl catalog") {
		t.Fatalf("help output=%q err=%v", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"completion", "bash"}, &out, &out); err != nil || !strings.Contains(out.String(), "complete -F _termctl_complete termctl") {
		t.Fatalf("completion output=%q err=%v", out.String(), err)
	}
}

func TestCatalogListUsesShippedConfig(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--config-dir", "../../configs", "catalog", "list"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ping") || !strings.Contains(out.String(), "trace_filtered") {
		t.Fatalf("catalog output missing new operations: %q", out.String())
	}
}
