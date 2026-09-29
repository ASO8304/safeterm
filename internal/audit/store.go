package audit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const maxAuditRecordBytes = 1 << 20

type Envelope struct {
	Sequence     uint64 `json:"sequence"`
	PreviousHash string `json:"previous_hash,omitempty"`
	Hash         string `json:"hash"`
	Event        Event  `json:"event"`
}

type FileStore struct {
	mu       sync.Mutex
	file     *os.File
	sequence uint64
	lastHash string
}

func OpenFileStore(path string) (*FileStore, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("audit: store path must be absolute")
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("audit: store path must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("audit: inspect store: %w", err)
	}
	envelopes, err := ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open store: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("audit: secure store permissions: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("audit: inspect opened store: %w", err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		file.Close()
		return nil, fmt.Errorf("audit: store path changed during open")
	}
	store := &FileStore{file: file}
	if len(envelopes) > 0 {
		last := envelopes[len(envelopes)-1]
		store.sequence, store.lastHash = last.Sequence, last.Hash
	}
	return store, nil
}

func (s *FileStore) Record(ctx context.Context, event Event) error {
	if s == nil || ctx == nil {
		return fmt.Errorf("audit: invalid store")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("audit: record canceled: %w", ctx.Err())
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return fmt.Errorf("audit: store is closed")
	}
	envelope := Envelope{Sequence: s.sequence + 1, PreviousHash: s.lastHash, Event: event}
	hash, err := envelopeHash(envelope)
	if err != nil {
		return err
	}
	envelope.Hash = hash
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("audit: encode record: %w", err)
	}
	encoded = append(encoded, '\n')
	written, err := s.file.Write(encoded)
	if err != nil || written != len(encoded) {
		_ = s.file.Close()
		s.file = nil
		return fmt.Errorf("audit: append record: wrote %d of %d bytes: %w", written, len(encoded), err)
	}
	if err := s.file.Sync(); err != nil {
		_ = s.file.Close()
		s.file = nil
		return fmt.Errorf("audit: sync record: %w", err)
	}
	s.sequence, s.lastHash = envelope.Sequence, envelope.Hash
	return nil
}

func (s *FileStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// ReadFile validates every sequence and hash link before returning records.
func ReadFile(path string) ([]Envelope, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var result []Envelope
	var previous string
	for sequence := uint64(1); ; sequence++ {
		line, readErr := readBoundedLine(reader)
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				return nil, fmt.Errorf("audit: incomplete record %d", sequence)
			}
			var envelope Envelope
			decoder := json.NewDecoder(bytes.NewReader(line))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&envelope); err != nil {
				return nil, fmt.Errorf("audit: decode record %d: %w", sequence, err)
			}
			var trailing any
			if err := decoder.Decode(&trailing); err != io.EOF {
				return nil, fmt.Errorf("audit: trailing data at record %d", sequence)
			}
			if envelope.Sequence != sequence || envelope.PreviousHash != previous {
				return nil, fmt.Errorf("audit: broken chain at record %d", sequence)
			}
			if err := envelope.Event.Validate(); err != nil {
				return nil, fmt.Errorf("audit: invalid event at record %d: %w", sequence, err)
			}
			expected, err := envelopeHash(envelope)
			if err != nil || expected != envelope.Hash {
				return nil, fmt.Errorf("audit: hash mismatch at record %d", sequence)
			}
			previous = envelope.Hash
			result = append(result, envelope)
		}
		if readErr == io.EOF {
			return result, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("audit: read store: %w", readErr)
		}
	}
}

func readBoundedLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxAuditRecordBytes {
			return nil, fmt.Errorf("audit: record exceeds size limit")
		}
		line = append(line, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, err
	}
}

func envelopeHash(envelope Envelope) (string, error) {
	envelope.Hash = ""
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("audit: hash record: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
