//go:build !linux

package configmanager

import "os"

func fileOwnerUID(os.FileInfo) (uint32, bool)   { return 0, false }
func fileOwnerIDs(os.FileInfo) (int, int, bool) { return 0, 0, false }
func fileLinkCount(os.FileInfo) (uint64, bool)  { return 0, false }
