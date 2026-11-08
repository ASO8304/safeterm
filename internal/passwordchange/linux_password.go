//go:build linux

package passwordchange

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Run the system PAM password tool with the target's real UID, so PAM requires
// the old password. Never run passwd with real UID 0, which bypasses that check.
// Secrets travel only over stdin; no shell, argv, files or captured output.
func ChangeLinuxPassword(ctx context.Context, a Account, current, next []byte) error {
	if a.UID == 0 || !validID.MatchString(a.Name) {
		return ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	input := make([]byte, 0, len(current)+2*len(next)+3)
	input = append(input, current...)
	input = append(input, '\n')
	input = append(input, next...)
	input = append(input, '\n')
	input = append(input, next...)
	input = append(input, '\n')
	defer clear(input)
	cmd := exec.CommandContext(ctx, "/usr/local/bin/safeterm-password-manager", "--linux-password-child", a.Name)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Linux password change denied: check the current password and the account's password policy")
	}
	return nil
}

// LinuxPasswordChild is reachable only by the root service. Keep the effective
// UID already held by that service, but use the operator's real UID: passwd/PAM
// then require the operator's current password. No privilege is gained at exec,
// so the service retains NoNewPrivileges=yes. stdin passes straight to PAM.
func LinuxPasswordChild(name string) error {
	if os.Getuid() != 0 || os.Geteuid() != 0 || !validID.MatchString(name) {
		return ErrDenied
	}
	a, err := LookupAccount(name)
	if err != nil {
		return ErrDenied
	}
	if err = unix.Setgroups([]int{}); err != nil {
		return err
	}
	if err = unix.Setresgid(int(a.GID), int(a.GID), int(a.GID)); err != nil {
		return err
	}
	if err = unix.Setresuid(int(a.UID), 0, 0); err != nil {
		return err
	}
	return unix.Exec("/usr/bin/passwd", []string{"passwd", "--", a.Name}, []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"})
}
