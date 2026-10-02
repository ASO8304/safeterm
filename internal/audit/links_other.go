//go:build !linux

package audit

import "os"

func singleLink(os.FileInfo) bool { return true }
