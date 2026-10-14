//go:build !linux

package identity

func CheckRootManaged(string, uint32) error { return ErrDenied }
