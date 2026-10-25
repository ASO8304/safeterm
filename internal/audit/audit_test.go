package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"safeterm/internal/contract"
)

func testEvent() Event {
	req, _ := contract.NewExecutionRequest("capture", map[string]string{"token": "top-secret", "host": "127.0.0.1"}, contract.Metadata{
		BrokerType: "cli", UserID: "alice", ClaimedRoles: []string{"netops"},
	})
	start := time.Now().UTC()
	response := &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultSuccess, ExitCode: 0, StartedAt: start, EndedAt: start.Add(15 * time.Millisecond)}
	return NewEvent(req, "1.0.0", ComponentRunner, "network-observer", "completed", response)
}

// TEST-AUDIT-001, TEST-AUDIT-002
func TestEventCapturesCorrelationAndExcludesArgumentValues(t *testing.T) {
	event := testEvent()
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if event.RequestID == "" || event.Operation != "capture" || event.UserID != "alice" || event.BrokerType != "cli" || event.DurationMillis == nil || *event.DurationMillis != 15 {
		t.Fatalf("incomplete event: %+v", event)
	}
	if strings.Join(event.ArgumentNames, ",") != "host,token" {
		t.Fatalf("argument names = %v", event.ArgumentNames)
	}
	encoded, _ := json.Marshal(event)
	if strings.Contains(string(encoded), "top-secret") || strings.Contains(string(encoded), "127.0.0.1") {
		t.Fatalf("argument value leaked: %s", encoded)
	}
}

// TEST-AUDIT-002
func TestRejectedEventUsesFailureStageWithoutExitCode(t *testing.T) {
	req, _ := contract.NewExecutionRequest("system_info", nil, contract.Metadata{})
	start := time.Now().UTC()
	response := &contract.ExecutionResponse{
		RequestID: req.RequestID, Result: contract.ResultRejected,
		FailureStage: "policy", StartedAt: start, EndedAt: start.Add(time.Millisecond),
	}
	event := NewEvent(req, "1.0.0", ComponentRunner, "diagnostics", "completed", response)
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if event.ExitCode != nil || event.FailureStage != "policy" {
		t.Fatalf("unexpected rejected event: %+v", event)
	}
}

// TEST-AUDIT-003, TEST-AUDIT-004
func TestFileStoreAppendsDurablyAndVerifiesChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	store, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := store.Record(context.Background(), testEvent()); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Sequence != 1 || entries[1].Sequence != 2 || entries[1].PreviousHash != entries[0].Hash {
		t.Fatalf("invalid chain: %+v", entries)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("store mode = %o", info.Mode().Perm())
	}
	reopened, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Record(context.Background(), testEvent()); err != nil {
		t.Fatal(err)
	}
}

// TEST-AUDIT-004
func TestFileStoreRejectsTamperingAndSymlink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "audit.jsonl")
	store, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(context.Background(), testEvent()); err != nil {
		t.Fatal(err)
	}
	store.Close()
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), "completed", "tampered!", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileStore(path); err == nil {
		t.Fatal("tampered store accepted")
	}
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, "link")); err == nil {
		if _, err := OpenFileStore(filepath.Join(directory, "link")); err == nil {
			t.Fatal("symlink store accepted")
		}
	}
}

// TEST-AUDIT-003
func TestFileStoreSerializesConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	store, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := store.Record(context.Background(), testEvent()); err != nil {
				t.Errorf("record: %v", err)
			}
		}()
	}
	wait.Wait()
	store.Close()
	entries, err := ReadFile(path)
	if err != nil || len(entries) != 20 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
}

// TEST-AUDIT-008
func TestEventAndStoreRejectInvalidInputs(t *testing.T) {
	if err := (Event{}).Validate(); err == nil {
		t.Fatal("empty event accepted")
	}
	if _, err := OpenFileStore("relative.jsonl"); err == nil {
		t.Fatal("relative store path accepted")
	}
	if err := (*FileStore)(nil).Record(context.Background(), testEvent()); err == nil {
		t.Fatal("nil store accepted")
	}
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	store, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err := store.Record(context.Background(), testEvent()); err == nil {
		t.Fatal("closed store accepted")
	}
	incomplete := filepath.Join(t.TempDir(), "incomplete.jsonl")
	if err := os.WriteFile(incomplete, []byte(`{"sequence":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(incomplete); err == nil {
		t.Fatal("incomplete record accepted")
	}
	oversized := filepath.Join(t.TempDir(), "oversized.jsonl")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", maxAuditRecordBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(oversized); err == nil {
		t.Fatal("oversized record accepted")
	}
}
