package integrity

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"safeterm/internal/config"
)

func signedTestCatalog() *config.Config {
	return &config.Config{
		Version: "test-v1",
		Profiles: []config.ProfileDefinition{{
			Name: config.ProfileDiagnostics, ExecutorKind: config.ExecutorProcess,
			SocketPath: "/run/safeterm/test.sock", RunAsUser: "test",
			Capabilities: []string{}, MemoryLimitMiB: 64, ProcessLimit: 8,
		}},
		Executables: []config.ExecutableDefinition{{
			ID: "tool", Path: "/usr/bin/tool",
			SHA256:          "0000000000000000000000000000000000000000000000000000000000000000",
			AllowedProfiles: []config.RunnerProfile{config.ProfileDiagnostics},
		}},
		Operations: []config.OperationDefinition{{
			Name: "probe", RunnerProfile: config.ProfileDiagnostics,
			ExecutorKind: config.ExecutorProcess, ExecutableRef: "tool",
		}},
		Roles:    []config.RoleDefinition{{Name: "operator"}, {Name: "viewer"}},
		Policies: []config.PolicyDefinition{{Role: "operator", Operations: []string{"probe"}}, {Role: "viewer", Operations: []string{}}},
	}
}

// TEST-INTEGRITY-009
func TestCatalogSignatureAcceptsExactCatalogAndRejectsMutation(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := signedTestCatalog()
	signature, err := SignCatalog(cfg, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCatalog(cfg, publicKey, signature); err != nil {
		t.Fatal(err)
	}
	cfg.Executables[0].SHA256 = "1111111111111111111111111111111111111111111111111111111111111111"
	if err := VerifyCatalog(cfg, publicKey, signature); err == nil {
		t.Fatal("mutated executable digest retained a valid signature")
	}
}

// TEST-INTEGRITY-010
func TestCanonicalCatalogIgnoresUnorderedCollectionOrder(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	original := signedTestCatalog()
	signature, err := SignCatalog(original, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	reordered := signedTestCatalog()
	reordered.Roles[0], reordered.Roles[1] = reordered.Roles[1], reordered.Roles[0]
	reordered.Policies[0], reordered.Policies[1] = reordered.Policies[1], reordered.Policies[0]
	if err := VerifyCatalog(reordered, publicKey, signature); err != nil {
		t.Fatalf("semantic reorder changed signature: %v", err)
	}
}

// TEST-INTEGRITY-011
func TestVerifyCatalogFilesFailsClosedForBadTrustMaterial(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	cfg := signedTestCatalog()
	signature, _ := SignCatalog(cfg, privateKey)
	dir := t.TempDir()
	publicPath := filepath.Join(dir, "catalog.pub")
	signaturePath := filepath.Join(dir, "catalog.sig")
	if err := os.WriteFile(publicPath, EncodeBase64(publicKey), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signaturePath, EncodeBase64(signature), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCatalogFiles(cfg, publicPath, signaturePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(publicPath, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCatalogFiles(cfg, publicPath, signaturePath); err == nil {
		t.Fatal("writable trust anchor accepted")
	}
	if err := os.Chmod(publicPath, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, public, signature string
	}{
		{"missing key", filepath.Join(dir, "missing"), signaturePath},
		{"bad key", signaturePath, signaturePath},
		{"missing signature", publicPath, filepath.Join(dir, "missing")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyCatalogFiles(cfg, tc.public, tc.signature); err == nil {
				t.Fatal("bad trust material accepted")
			}
		})
	}
}

func TestCatalogSigningRejectsInvalidInputs(t *testing.T) {
	if _, err := SignCatalog(signedTestCatalog(), ed25519.PrivateKey("short")); err == nil {
		t.Fatal("short private key accepted")
	}
	if err := VerifyCatalog(signedTestCatalog(), ed25519.PublicKey("short"), make([]byte, ed25519.SignatureSize)); err == nil {
		t.Fatal("short public key accepted")
	}
	if err := VerifyCatalog(signedTestCatalog(), make([]byte, ed25519.PublicKeySize), []byte("short")); err == nil {
		t.Fatal("short signature accepted")
	}
}

// TEST-INTEGRITY-012
func TestShippedCatalogHasValidDetachedSignature(t *testing.T) {
	cfg, err := config.LoadConfig("../defaults/catalog")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	publicKey := filepath.Join(dir, "catalog.pub")
	signature := filepath.Join(dir, "catalog.sig")
	for source, target := range map[string]string{
		"../defaults/catalog/trust/catalog-ed25519.pub": publicKey,
		"../defaults/catalog/catalog.sig":               signature,
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := VerifyCatalogFiles(cfg, publicKey, signature); err != nil {
		t.Fatalf("shipped catalog signature: %v", err)
	}
}
