//go:build !linux

package execution

import (
	"os"
	"os/exec"
)

func configureProcessGroup(_ *exec.Cmd) {}

func interruptProcessGroup(process *os.Process) error {
	return process.Signal(os.Interrupt)
}

func terminateProcessGroup(process *os.Process) error {
	return process.Kill()
}

func killProcessGroup(process *os.Process) error {
	return process.Kill()
}
