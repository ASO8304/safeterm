// Package integrity binds configured executable paths to approved file hashes
// immediately before process creation. Verified files are executed through an
// open /proc file descriptor so a path replacement cannot change the inode
// selected for execution.
package integrity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"safeterm/internal/agent"
	"safeterm/internal/config"
)

const exitCodeUnavailable = -1

type binding struct {
	canonicalPath string
	digest        [sha256.Size]byte
}

// Executor verifies an allowlisted executable and delegates process handling
// while keeping the verified file descriptor open for the whole execution.
type Executor struct {
	delegate agent.Executor
	bindings map[string]binding
}

// NewExecutor constructs a profile-scoped integrity boundary. It verifies the
// initial files at startup and saves their canonical paths. Runtime execution
// repeats every check and rejects path, symlink, inode, or content changes.
func NewExecutor(cfg *config.Config, profile config.RunnerProfile, delegate agent.Executor) (*Executor, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("integrity: executable pinning requires Linux /proc")
	}
	if delegate == nil {
		return nil, fmt.Errorf("integrity: delegate executor is required")
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("integrity: invalid catalog: %w", err)
	}
	profileExists := false
	for _, definition := range cfg.Profiles {
		if definition.Name == profile {
			profileExists = true
			break
		}
	}
	if !profileExists {
		return nil, fmt.Errorf("integrity: unknown profile %q", profile)
	}

	bindings := make(map[string]binding)
	for _, executable := range cfg.Executables {
		if !containsProfile(executable.AllowedProfiles, profile) {
			continue
		}
		digestBytes, err := hex.DecodeString(executable.SHA256)
		if err != nil || len(digestBytes) != sha256.Size {
			return nil, fmt.Errorf("integrity: executable %q has an invalid SHA-256 digest", executable.ID)
		}
		var digest [sha256.Size]byte
		copy(digest[:], digestBytes)
		canonical, err := filepath.EvalSymlinks(executable.Path)
		if err != nil {
			return nil, fmt.Errorf("integrity: resolve executable %q: %w", executable.ID, err)
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			return nil, fmt.Errorf("integrity: canonicalize executable %q: %w", executable.ID, err)
		}
		candidate := binding{canonicalPath: filepath.Clean(canonical), digest: digest}
		file, err := verifyOpen(executable.Path, candidate)
		if err != nil {
			return nil, fmt.Errorf("integrity: verify executable %q: %w", executable.ID, err)
		}
		file.Close()
		bindings[executable.Path] = candidate
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("integrity: profile %q has no approved executables", profile)
	}
	return &Executor{delegate: delegate, bindings: bindings}, nil
}

// Execute fails closed unless command exactly matches a profile-scoped
// registry path and the current canonical file still has the approved digest.
func (e *Executor) Execute(ctx context.Context, command string, args []string) (string, string, int, error) {
	if e == nil || e.delegate == nil || ctx == nil {
		return "", "", exitCodeUnavailable, fmt.Errorf("integrity: invalid executor call")
	}
	file, err := e.openVerified(command)
	if err != nil {
		return "", "", exitCodeUnavailable, err
	}
	defer file.Close()

	// The child resolves this path to the already-open, verified inode before
	// close-on-exec is applied. Keeping the descriptor open removes the path
	// replacement window between hashing and execve for native Linux binaries.
	pinnedPath := fmt.Sprintf("/proc/self/fd/%d", file.Fd())
	return e.delegate.Execute(ctx, pinnedPath, args)
}

// VerifyExecutable repeats the profile-scoped integrity check without running
// the file. Typed controllers use it for fixed executable dependencies such as
// systemctl that are launched by the directly executed sudo controller.
func (e *Executor) VerifyExecutable(command string) error {
	if e == nil || e.delegate == nil {
		return fmt.Errorf("integrity: invalid executor call")
	}
	file, err := e.openVerified(command)
	if err != nil {
		return err
	}
	return file.Close()
}

func (e *Executor) openVerified(command string) (*os.File, error) {
	binding, ok := e.bindings[command]
	if !ok || command == "" {
		return nil, fmt.Errorf("integrity: executable path is not approved for this profile")
	}
	file, err := verifyOpen(command, binding)
	if err != nil {
		return nil, fmt.Errorf("integrity: verify %q: %w", command, err)
	}
	return file, nil
}

func verifyOpen(configuredPath string, approved binding) (*os.File, error) {
	canonical, err := filepath.EvalSymlinks(configuredPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, fmt.Errorf("canonicalize path: %w", err)
	}
	canonical = filepath.Clean(canonical)
	if canonical != approved.canonicalPath {
		return nil, fmt.Errorf("canonical path changed")
	}

	file, err := os.Open(canonical)
	if err != nil {
		return nil, fmt.Errorf("open canonical file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat open file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("approved executable must be a regular executable file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("approved executable must not be group- or world-writable")
	}
	var magic [4]byte
	if _, err := file.ReadAt(magic[:], 0); err != nil || magic != [4]byte{0x7f, 'E', 'L', 'F'} {
		return nil, fmt.Errorf("approved executable must be a native ELF binary")
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, fmt.Errorf("hash executable: %w", err)
	}
	actual := hasher.Sum(nil)
	if !equalDigest(actual, approved.digest[:]) {
		return nil, fmt.Errorf("SHA-256 mismatch")
	}

	currentCanonical, err := filepath.EvalSymlinks(configuredPath)
	if err != nil {
		return nil, fmt.Errorf("re-resolve path: %w", err)
	}
	currentInfo, err := os.Stat(currentCanonical)
	if err != nil {
		return nil, fmt.Errorf("re-stat path: %w", err)
	}
	if filepath.Clean(currentCanonical) != approved.canonicalPath || !os.SameFile(info, currentInfo) {
		return nil, fmt.Errorf("executable changed during verification")
	}
	ok = true
	return file, nil
}

func equalDigest(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for i := range left {
		different |= left[i] ^ right[i]
	}
	return different == 0
}

func containsProfile(items []config.RunnerProfile, wanted config.RunnerProfile) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}
