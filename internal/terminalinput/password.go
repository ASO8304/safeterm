// Package terminalinput provides secret-safe administrator terminal input.
package terminalinput

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ReadConfirmed disables echo before publishing either prompt. ReadPassword's
// own mode switch happens after the caller's prompt and otherwise races fast
// input/paste. The outer terminal state is restored on every normal/error return.
func ReadConfirmed(input *os.File, out io.Writer, prompt string) ([]byte, error) {
	fd := int(input.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf("an interactive terminal is required to set a password")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer term.Restore(fd, state)
	if _, err = fmt.Fprint(out, prompt); err != nil {
		return nil, err
	}
	first, err := term.ReadPassword(fd)
	fmt.Fprint(out, "\r\n")
	if err != nil {
		clear(first)
		return nil, err
	}
	if _, err = fmt.Fprint(out, "Confirm password: "); err != nil {
		clear(first)
		return nil, err
	}
	second, err := term.ReadPassword(fd)
	fmt.Fprint(out, "\r\n")
	match := bytes.Equal(first, second)
	clear(second)
	if err != nil {
		clear(first)
		return nil, err
	}
	if !match {
		clear(first)
		return nil, fmt.Errorf("passwords do not match")
	}
	return first, nil
}
