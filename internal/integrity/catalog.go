package integrity

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"safeterm/internal/config"
)

// CanonicalCatalog returns the deterministic bytes covered by a catalog
// signature. Semantically unordered collections are sorted on a deep copy;
// argument templates retain their security-sensitive token order.
func CanonicalCatalog(cfg *config.Config) ([]byte, error) {
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("integrity: invalid catalog: %w", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("integrity: clone catalog: %w", err)
	}
	var canonical config.Config
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return nil, fmt.Errorf("integrity: decode catalog clone: %w", err)
	}
	sort.Slice(canonical.Operations, func(i, j int) bool { return canonical.Operations[i].Name < canonical.Operations[j].Name })
	for i := range canonical.Operations {
		sort.Slice(canonical.Operations[i].Parameters, func(a, b int) bool {
			return canonical.Operations[i].Parameters[a].Name < canonical.Operations[i].Parameters[b].Name
		})
	}
	sort.Slice(canonical.Executables, func(i, j int) bool { return canonical.Executables[i].ID < canonical.Executables[j].ID })
	for i := range canonical.Executables {
		sort.Slice(canonical.Executables[i].AllowedProfiles, func(a, b int) bool {
			return canonical.Executables[i].AllowedProfiles[a] < canonical.Executables[i].AllowedProfiles[b]
		})
	}
	sort.Slice(canonical.Profiles, func(i, j int) bool { return canonical.Profiles[i].Name < canonical.Profiles[j].Name })
	for i := range canonical.Profiles {
		sort.Strings(canonical.Profiles[i].Capabilities)
	}
	sort.Slice(canonical.Roles, func(i, j int) bool { return canonical.Roles[i].Name < canonical.Roles[j].Name })
	for i := range canonical.Policies {
		sort.Strings(canonical.Policies[i].Operations)
	}
	sort.Slice(canonical.Policies, func(i, j int) bool {
		if canonical.Policies[i].Role == canonical.Policies[j].Role {
			return strings.Join(canonical.Policies[i].Operations, "\x00") < strings.Join(canonical.Policies[j].Operations, "\x00")
		}
		return canonical.Policies[i].Role < canonical.Policies[j].Role
	})
	return json.Marshal(canonical)
}

func SignCatalog(cfg *config.Config, privateKey ed25519.PrivateKey) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("integrity: invalid Ed25519 private key length")
	}
	message, err := CanonicalCatalog(cfg)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(privateKey, message), nil
}

func VerifyCatalog(cfg *config.Config, publicKey ed25519.PublicKey, signature []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("integrity: invalid Ed25519 public key length")
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("integrity: invalid Ed25519 signature length")
	}
	message, err := CanonicalCatalog(cfg)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("integrity: catalog signature verification failed")
	}
	return nil
}

// VerifyCatalogFiles reads a separately provisioned trust anchor and detached
// signature. Both files contain standard raw-base64 values with optional
// surrounding whitespace.
func VerifyCatalogFiles(cfg *config.Config, publicKeyPath, signaturePath string) error {
	publicKey, publicInfo, err := readBase64FileInfo(publicKeyPath, ed25519.PublicKeySize)
	if err != nil {
		return fmt.Errorf("integrity: read catalog public key: %w", err)
	}
	if publicInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("integrity: catalog public key must not be group- or world-writable")
	}
	signature, signatureInfo, err := readBase64FileInfo(signaturePath, ed25519.SignatureSize)
	if err != nil {
		return fmt.Errorf("integrity: read catalog signature: %w", err)
	}
	if signatureInfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("integrity: catalog signature must not be group- or world-writable")
	}
	return VerifyCatalog(cfg, ed25519.PublicKey(publicKey), signature)
}

func ReadPrivateKey(path string) (ed25519.PrivateKey, error) {
	key, info, err := readBase64FileInfo(path, ed25519.PrivateKeySize)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("private key must have no group or world permissions")
	}
	return ed25519.PrivateKey(key), nil
}

func EncodeBase64(value []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(value) + "\n")
}

func readBase64FileInfo(path string, expectedSize int) ([]byte, os.FileInfo, error) {
	if path == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("file must be regular and not a symlink")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	openInfo, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, openInfo) {
		return nil, nil, fmt.Errorf("file changed while opening")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("decode base64: %w", err)
	}
	if len(decoded) != expectedSize {
		return nil, nil, fmt.Errorf("decoded value has length %d, want %d", len(decoded), expectedSize)
	}
	return decoded, openInfo, nil
}
