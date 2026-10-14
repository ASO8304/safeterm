//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

func preserveIdentityOwner(file *os.File, target os.FileInfo) error {
	want, ok := target.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot inspect target ownership")
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	got, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot inspect temporary file ownership")
	}
	if got.Uid == want.Uid && got.Gid == want.Gid {
		return nil
	}
	return file.Chown(int(want.Uid), int(want.Gid))
}
