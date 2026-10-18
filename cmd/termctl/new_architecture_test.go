package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRetiredMutationCommandsCannotBeUsed(t *testing.T) {
	var out bytes.Buffer
	for _, args := range [][]string{{"catalog", "apply"}, {"catalog", "rollback"}, {"catalog", "checkpoint"}, {"catalog", "prepare"}, {"catalog", "regenerate-hashes"}, {"catalog", "export"}, {"config", "check"}, {"config", "apply"}, {"config", "rollback"}, {"config", "checkpoint"}} {
		if err := run(args, &out, &out); err == nil {
			t.Fatalf("retired command accepted: %v", args)
		}
	}
}

func TestHelpDoesNotAdvertiseRemovedConfigMutations(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"config apply", "catalog apply", "--catalog-source", "RESOURCE | check | apply"} {
		if strings.Contains(out.String(), old) {
			t.Fatalf("obsolete help: %s", old)
		}
	}
}
