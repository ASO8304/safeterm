//go:build !linux

package main

import (
	"fmt"
	"io"
)

func resourceCommand(string, []string, io.Writer) error {
	return fmt.Errorf("resource activation requires Linux")
}
