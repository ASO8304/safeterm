package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"safeterm/internal/config"
	"safeterm/internal/integrity"
)

func main() {
	generate := flag.Bool("generate-key", false, "generate a new Ed25519 key pair")
	configDir := flag.String("config", "", "catalog directory to sign")
	privateKeyPath := flag.String("private-key", "", "base64 Ed25519 private key path")
	publicKeyPath := flag.String("public-key", "", "base64 Ed25519 public key output path")
	signaturePath := flag.String("signature", "", "detached base64 signature output path")
	flag.Parse()
	if err := run(*generate, *configDir, *privateKeyPath, *publicKeyPath, *signaturePath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(generate bool, configDir, privateKeyPath, publicKeyPath, signaturePath string) error {
	if generate {
		if privateKeyPath == "" || publicKeyPath == "" {
			return fmt.Errorf("catalog-sign: -private-key and -public-key are required for key generation")
		}
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("catalog-sign: generate key: %w", err)
		}
		if err := writeExclusive(privateKeyPath, integrity.EncodeBase64(privateKey), 0o600); err != nil {
			return fmt.Errorf("catalog-sign: write private key: %w", err)
		}
		if err := writeExclusive(publicKeyPath, integrity.EncodeBase64(publicKey), 0o644); err != nil {
			os.Remove(privateKeyPath)
			return fmt.Errorf("catalog-sign: write public key: %w", err)
		}
		return nil
	}
	if configDir == "" || privateKeyPath == "" || signaturePath == "" {
		return fmt.Errorf("catalog-sign: -config, -private-key, and -signature are required for signing")
	}
	cfg, err := config.LoadConfig(configDir)
	if err != nil {
		return err
	}
	privateKey, err := integrity.ReadPrivateKey(privateKeyPath)
	if err != nil {
		return fmt.Errorf("catalog-sign: read private key: %w", err)
	}
	signature, err := integrity.SignCatalog(cfg, privateKey)
	if err != nil {
		return err
	}
	if err := writeAtomic(signaturePath, integrity.EncodeBase64(signature), 0o644); err != nil {
		return fmt.Errorf("catalog-sign: write signature: %w", err)
	}
	return nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("output path exists and is not a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".catalog-sign-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	return file.Close()
}
