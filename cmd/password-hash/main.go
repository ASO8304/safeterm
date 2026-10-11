package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"safeterm/internal/broker"
)

func main() {
	stdin := flag.Bool("stdin", false, "read one password line from stdin instead of the terminal")
	output := flag.String("output", "", "create a mode-0600 hash file (default: stdout)")
	flag.Parse()
	if err := run(*stdin, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stdin bool, output string) error {
	var password []byte
	var err error
	if stdin {
		password, err = io.ReadAll(io.LimitReader(os.Stdin, 75))
		if err != nil {
			return fmt.Errorf("password-hash: cannot read password")
		}
		password = bytes.TrimSuffix(password, []byte("\n"))
		password = bytes.TrimSuffix(password, []byte("\r"))
	} else {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("password-hash: a terminal is required; use -stdin for piped input")
		}
		fmt.Fprint(os.Stderr, "Password: ")
		password, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("password-hash: cannot read password")
		}
		defer clear(password)
		fmt.Fprint(os.Stderr, "Confirm password: ")
		confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("password-hash: cannot read confirmation")
		}
		match := bytes.Equal(password, confirmation)
		clear(confirmation)
		if !match {
			return fmt.Errorf("password-hash: passwords do not match")
		}
	}
	defer clear(password)
	if len(password) == 0 || len(password) > 72 {
		return fmt.Errorf("password-hash: password must contain 1 through 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword(password, broker.PasswordHashCost)
	if err != nil {
		return fmt.Errorf("password-hash: hashing failed")
	}
	hash = append(hash, '\n')
	if output == "" {
		_, err = os.Stdout.Write(hash)
		return err
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("password-hash: cannot create output: %w", err)
	}
	_, err = file.Write(hash)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
