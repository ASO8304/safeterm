//go:build !linux

package identity

func readLocalUsers(string) ([]byte, error) { return nil, ErrDenied }
