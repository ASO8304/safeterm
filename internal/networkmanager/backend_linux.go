//go:build linux

package networkmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Trial interface {
	Ready(context.Context) error
	Finish(bool) error
	Done() <-chan struct{}
}
type Backend interface {
	Validate(context.Context, []File, map[string][]byte) error
	Begin(context.Context, int) (Trial, error)
	Restore(context.Context) error
	Status(context.Context, []string) (string, error)
}
type Netplan struct{ Root string }
type limitedOutput struct {
	mu   sync.Mutex
	data []byte
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.data) < 65536 {
		n := min(len(p), 65536-len(b.data))
		b.data = append(b.data, p[:n]...)
	}
	return len(p), nil
}
func (b *limitedOutput) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
func command(ctx context.Context, program string, args ...string) (string, error) {
	switch program {
	case "/usr/sbin/netplan", "/usr/sbin/ip", "/usr/bin/resolvectl":
	default:
		return "", fmt.Errorf("network: unsupported executable")
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out := &limitedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("network: %s failed: %w: %s", filepath.Base(program), err, out.String())
	}
	return out.String(), nil
}
func (b *Netplan) Validate(ctx context.Context, files []File, changed map[string][]byte) error {
	if b.Root == "/" {
		if err := supportedHost(); err != nil {
			return err
		}
	}

	stage, err := os.MkdirTemp("", "safeterm-netplan-validate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, file := range files {
		path := filepath.Join(stage, file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		data := file.Data
		if value, ok := changed[file.Path]; ok {
			data = value
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = command(ctx, "/usr/sbin/netplan", "generate", "--root-dir", stage)
	return err
}

type netplanTrial struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan struct{}
	output *limitedOutput
	err    error
	mu     sync.Mutex
	root   string
}

func (b *Netplan) Begin(_ context.Context, seconds int) (Trial, error) {
	// A pipe is deliberate: the privileged process is independent from the user's
	// PTY. Keep stdin open, confirm with SIGUSR1 only after durable file commit.
	if b.Root != "/" {
		return nil, fmt.Errorf("network: real apply is forbidden on a redirected filesystem root")
	}
	if _, err := os.Stat("/run/netplan/netplan-try.ready"); err == nil {
		return nil, fmt.Errorf("network: another netplan try is active")
	}
	cmd := exec.Command("/usr/sbin/netplan", "try", "--timeout", strconv.Itoa(seconds))
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out := &limitedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, err
	}
	t := &netplanTrial{cmd: cmd, stdin: stdin, done: make(chan struct{}), output: out, root: b.Root}
	go func() {
		err := cmd.Wait()
		t.mu.Lock()
		t.err = err
		t.mu.Unlock()
		close(t.done)
		stdin.Close()
	}()
	return t, nil
}
func (t *netplanTrial) Done() <-chan struct{} { return t.done }
func (t *netplanTrial) Ready(ctx context.Context) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.done:
			return fmt.Errorf("network: netplan try exited before readiness: %s", t.output.String())
		case <-tick.C:
			if _, err := os.Stat("/run/netplan/netplan-try.ready"); err == nil {
				return nil
			}
		}
	}
}
func (t *netplanTrial) Finish(accept bool) error {
	select {
	case <-t.done:
		if accept {
			return fmt.Errorf("network: trial exited before confirmation")
		}
		return nil
	default:
	}
	sig := syscall.SIGINT
	if accept {
		sig = syscall.SIGUSR1
	}
	if err := t.cmd.Process.Signal(sig); err != nil {
		return err
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-t.done:
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.err != nil {
			return fmt.Errorf("network: trial failed: %s", t.output.String())
		}
		if accept && !strings.Contains(t.output.String(), "Configuration accepted.") {
			return fmt.Errorf("network: trial did not acknowledge confirmation")
		}
		return nil
	case <-timer.C:
		_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
		<-t.done
		return fmt.Errorf("network: timed out finishing trial")
	}
}
func (b *Netplan) Restore(ctx context.Context) error {
	if b.Root != "/" {
		return fmt.Errorf("network: real apply is forbidden on a redirected filesystem root")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, err := command(ctx, "/usr/sbin/netplan", "apply")
	return err
}
func PhysicalInterfaces() map[string]string {
	out := map[string]string{}
	entries, _ := os.ReadDir("/sys/class/net")
	for _, entry := range entries {
		if entry.Name() == "lo" {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/sys/class/net", entry.Name(), "address"))
		if err == nil {
			out[entry.Name()] = strings.TrimSpace(string(data))
		}
	}
	return out
}
func (b *Netplan) Status(ctx context.Context, names []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := command(ctx, "/usr/sbin/ip", "-j", "address", "show")
	if err != nil {
		return "", err
	}
	var items []map[string]any
	if err = json.Unmarshal([]byte(data), &items); err != nil {
		return "", err
	}
	selected := []map[string]any{}
	for _, item := range items {
		if name, ok := item["ifname"].(string); ok && contains(names, name) {
			selected = append(selected, item)
		}
	}
	routes, err := command(ctx, "/usr/sbin/ip", "-j", "route", "show", "table", "all")
	if err != nil {
		return "", err
	}
	var routeItems []map[string]any
	if err = json.Unmarshal([]byte(routes), &routeItems); err != nil {
		return "", err
	}
	selectedRoutes := []map[string]any{}
	for _, item := range routeItems {
		if name, ok := item["dev"].(string); ok && contains(names, name) {
			selectedRoutes = append(selectedRoutes, item)
		}
	}
	dns := map[string]string{}
	for _, name := range names {
		result, err := command(ctx, "/usr/bin/resolvectl", "dns", name)
		if err != nil {
			dns[name] = "resolver unavailable"
		} else {
			dns[name] = strings.TrimSpace(result)
		}
	}
	encoded, _ := json.MarshalIndent(map[string]any{"interfaces": selected, "routes": selectedRoutes, "dns": dns}, "", "  ")
	return string(encoded), nil
}

func supportedHost() error {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = strings.Trim(value, "\"'")
		}
	}
	if values["ID"] != "ubuntu" || values["VERSION_ID"] != "24.04" {
		return fmt.Errorf("network: live edits require Ubuntu Server 24.04")
	}
	return nil
}
