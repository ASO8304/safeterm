//go:build !linux

package main

import (
	"fmt"
	"io"
)

func catalogManageCommand(string, []string, io.Writer) error {
	return fmt.Errorf("catalog activation requires Linux")
}

func configManageCommand(string, []string, io.Writer) error {
	return fmt.Errorf("config administration requires Linux")
}
