package main

import (
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStdinCreatesProtectedSaltedHashAndRefusesOverwrite(t *testing.T) {
	original := os.Stdin
	defer func() { os.Stdin = original }()
	feed := func(value string) {
		t.Helper()
		input, err := os.CreateTemp(t.TempDir(), "input")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := input.WriteString(value); err != nil {
			t.Fatal(err)
		}
		if _, err := input.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { input.Close() })
		os.Stdin = input
	}
	path := filepath.Join(t.TempDir(), "password.hash")
	feed(" test password \n")
	if err := run(true, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(string(data))), []byte(" test password ")); err != nil {
		t.Fatal("hash does not preserve password spaces")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("hash file is accessible by other users")
	}
	feed("replacement\n")
	if err := run(true, path); err == nil {
		t.Fatal("existing hash overwritten")
	}
	for _, value := range []string{"\n", strings.Repeat("x", 73) + "\n"} {
		feed(value)
		if err := run(true, filepath.Join(t.TempDir(), "invalid.hash")); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
}
