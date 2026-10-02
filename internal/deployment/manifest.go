package deployment

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const manifestName = "SHA256SUMS"

func verifyManifest(root string) error {
	manifestPath := filepath.Join(root, manifestName)
	if err := requireSecureFile(manifestPath, false); err != nil {
		return fmt.Errorf("deployment: checksum manifest: %w", err)
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > 4<<20 {
		return fmt.Errorf("deployment: checksum manifest must be between 1 byte and 4 MiB")
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()

	want := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(file, 4<<20))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return fmt.Errorf("deployment: malformed checksum manifest line")
		}
		digest := line[:64]
		if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size || digest != strings.ToLower(digest) {
			return fmt.Errorf("deployment: invalid checksum digest")
		}
		name := line[66:]
		if name == "" || strings.Contains(name, "\\") || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) != name || strings.HasPrefix(name, "../") {
			return fmt.Errorf("deployment: unsafe checksum path %q", name)
		}
		if _, exists := want[name]; exists {
			return fmt.Errorf("deployment: duplicate checksum path %q", name)
		}
		want[name] = digest
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("deployment: read checksum manifest: %w", err)
	}
	if len(want) == 0 {
		return fmt.Errorf("deployment: checksum manifest is empty")
	}

	actual := make([]string, 0, len(want))
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("deployment: bundle contains symlink %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("deployment: bundle contains non-regular file %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == manifestName {
			return nil
		}
		actual = append(actual, name)
		digest, exists := want[name]
		if !exists {
			return fmt.Errorf("deployment: unlisted bundle file %q", name)
		}
		value, err := fileDigest(path)
		if err != nil {
			return err
		}
		if value != digest {
			return fmt.Errorf("deployment: checksum mismatch for %q", name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(actual)
	if len(actual) != len(want) {
		return fmt.Errorf("deployment: checksum manifest lists missing files")
	}
	return nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
