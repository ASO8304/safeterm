//go:build !linux

package configmanager

import "fmt"

const WriterSocket = ""

type FileWriterClient struct {
	Socket string
	UID    uint32
}

func (c *FileWriterClient) Read(string) ([]byte, error) {
	return nil, fmt.Errorf("privileged writer requires Linux")
}
func (c *FileWriterClient) Commit(Patch, string) (string, error) {
	return "", fmt.Errorf("privileged writer requires Linux")
}
func (c *FileWriterClient) Restore(string, string, string) error {
	return fmt.Errorf("privileged writer requires Linux")
}
func (c *FileWriterClient) Latest(string) (string, error) {
	return "", fmt.Errorf("privileged writer requires Linux")
}
func WriterUnitDropIn(*Registry, bool) ([]byte, error) {
	return nil, fmt.Errorf("privileged writer requires Linux")
}
