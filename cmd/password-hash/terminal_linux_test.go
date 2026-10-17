//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPasswordHashTerminalChild(t *testing.T) {
	if os.Getenv("SAFETERM_PASSWORD_TERMINAL_CHILD") != "1" {
		return
	}
	if err := run(false, os.Getenv("SAFETERM_PASSWORD_TERMINAL_OUTPUT")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestTerminalPasswordIsHiddenConfirmedAndValidated(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required only for the PTY test harness")
	}
	for _, tc := range []struct {
		name, password, confirmation string
		failure                      bool
	}{
		{"valid", "DemoStrong42", "DemoStrong42", false},
		{"mismatch", "DemoStrong42", "Different42", true},
		{"complexity", "short", "short", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bootstrap.hash")
			cmd := exec.Command(python, "-c", `
import errno, os, select, subprocess, sys, time
master, slave = os.openpty()
env = dict(os.environ, SAFETERM_PASSWORD_TERMINAL_CHILD='1', SAFETERM_PASSWORD_TERMINAL_OUTPUT=sys.argv[2])
p = subprocess.Popen([sys.argv[1], '-test.run=^TestPasswordHashTerminalChild$'], stdin=slave, stdout=slave, stderr=slave, env=env)
os.close(slave)
data=b''
def until(marker):
    global data
    end=time.monotonic()+15
    while marker not in data:
        if time.monotonic()>end: raise RuntimeError('terminal prompt timed out')
        if not select.select([master],[],[],.1)[0]: continue
        chunk=os.read(master,4096)
        if not chunk: raise RuntimeError('terminal closed early')
        data+=chunk
try:
    until(b'Password: ')
    os.write(master, sys.argv[3].encode()+b'\n')
    until(b'Confirm password: ')
    os.write(master, sys.argv[4].encode()+b'\n')
    rc=p.wait(timeout=15)
    while select.select([master],[],[],0)[0]:
        try: chunk=os.read(master,4096)
        except OSError as e:
            if e.errno==errno.EIO: break
            raise
        if not chunk: break
        data+=chunk
    assert sys.argv[3].encode() not in data and sys.argv[4].encode() not in data, 'password echoed in terminal'
    assert (rc!=0)==(sys.argv[5]=='true'), 'unexpected helper result'
finally:
    if p.poll() is None: p.kill(); p.wait()
    os.close(master)
`, os.Args[0], path, tc.password, tc.confirmation, fmt.Sprint(tc.failure))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("PTY harness: %s %v", out, err)
			}
			info, err := os.Stat(path)
			if tc.failure {
				if !os.IsNotExist(err) {
					t.Fatal("invalid enrollment persisted hash")
				}
				return
			}
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("unsafe hash file: %v %v", info, err)
			}
		})
	}
}
