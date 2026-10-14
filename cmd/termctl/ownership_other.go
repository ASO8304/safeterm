//go:build !linux

package main

import (
	"fmt"
	"os"
)

func preserveIdentityOwner(_ *os.File, _ os.FileInfo) error {
	return fmt.Errorf("identity ownership preservation requires Linux")
}
