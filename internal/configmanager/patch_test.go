package configmanager

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAtomicBatchPreservesCommentsAndRejectsInvalidOrStaleChanges(t *testing.T) {
	ctx := context.Background()
	path := writeTestINI(t)
	registry := testRegistry(path, false)
	m, err := NewManager("application", registry, nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := actor("alice", "viewer", "operator")
	read, _ := m.Read(ctx, a, "cats-options")
	before, _ := os.ReadFile(path)
	p := Patch{ResourceID: "cats-options", BaseVersion: read.Version, Changes: []FieldChange{{Section: "cats", Key: "Cat_Port", Value: "100"}, {Section: "cats", Key: "Password", Value: "new-secret"}}}
	preview, err := m.Preview(ctx, a, p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview.Diff, "new-secret") || strings.Contains(preview.Diff, "hidden") {
		t.Fatal("secret leaked")
	}
	// Callers cannot mutate a prepared patch through its slice.
	p.Changes[0].Value = "101"
	if _, err = m.Apply(ctx, a, preview.Token); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	expected := strings.Replace(strings.Replace(string(before), "\"99\"", "\"100\"", 1), "\"hidden\"", "\"new-secret\"", 1)
	if string(after) != expected {
		t.Fatalf("unexpected file: %s", after)
	}
	if _, err = m.Rollback(ctx, a, "cats-options"); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != string(before) {
		t.Fatal("batch rollback did not restore whole file")
	}
	read, _ = m.Read(ctx, a, "cats-options")
	for _, changes := range [][]FieldChange{
		{{Section: "cats", Key: "Cat_Port", Value: "100"}, {Section: "cats", Key: "Immutable", Value: "bad"}},
		{{Section: "cats", Key: "Cat_Port", Value: "100"}, {Section: "cats", Key: "Cat_Port", Value: "101"}},
		{{Section: "cats", Key: "Cat_Port", Value: "100"}, {Section: "cats", Key: "Unknown", Value: "bad"}},
	} {
		if _, err = m.Preview(ctx, a, Patch{ResourceID: "cats-options", BaseVersion: read.Version, Changes: changes}); err == nil {
			t.Fatal("invalid batch accepted")
		}
		got, _ := os.ReadFile(path)
		if string(got) != string(before) {
			t.Fatal("failed batch changed file")
		}
	}
	p.BaseVersion = read.Version
	p.Changes[0].Value = "100"
	preview, err = m.Preview(ctx, a, p)
	if err != nil {
		t.Fatal(err)
	}
	external := append(append([]byte(nil), before...), []byte("; external change\n")...)
	if err = os.WriteFile(path, external, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Apply(ctx, a, preview.Token); err == nil {
		t.Fatal("stale batch accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(external) {
		t.Fatal("overwrote external edit")
	}
}

func TestBatchCrossFieldValidationUsesFinalDocument(t *testing.T) {
	r := Resource{Format: FormatINI, Fields: []FieldSchema{{Section: "limits", Key: "Min", Type: FieldInteger}, {Section: "limits", Key: "Max", Type: FieldInteger}}, CrossFieldRules: []CrossFieldRule{{Type: "less_or_equal", Section: "limits", Key: "Min", OtherSection: "limits", OtherKey: "Max"}}}
	data := []byte("; keep\n[limits]\nMin = 1\nMax = 2\n")
	p := Patch{Changes: []FieldChange{{Section: "limits", Key: "Min", Value: "3"}, {Section: "limits", Key: "Max", Value: "4"}}}
	rendered, _, _, _, err := renderPatch(r, data, p)
	if err != nil || string(rendered) != "; keep\n[limits]\nMin = 3\nMax = 4\n" {
		t.Fatalf("render=%q err=%v", rendered, err)
	}
	p.Key = "Min"
	if _, _, _, _, err = renderPatch(r, data, p); err == nil {
		t.Fatal("mixed batch accepted")
	}
	p.Key = ""
	p.Changes = make([]FieldChange, MaxPatchChanges+1)
	if _, _, _, _, err = renderPatch(r, data, p); err == nil {
		t.Fatal("oversized batch accepted")
	}
}

func TestEmptyGeneratedRegistryDeniesAllConfigResources(t *testing.T) {
	m, err := NewManager("application", &Registry{Version: "resources-v2", Resources: []Resource{}}, nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := actor("alice", "operator", "viewer")
	items, err := m.List(context.Background(), a)
	if err != nil || len(items) != 0 {
		t.Fatalf("empty registry list=%v err=%v", items, err)
	}
	if _, err = m.Read(context.Background(), a, "cats-options"); err == nil {
		t.Fatal("empty generation exposed old resource")
	}
	if _, err = ParseRegistry([]byte(`{"version":"old","resources":[],"require_separate_approval":true}`)); err == nil {
		t.Fatal("ignored legacy property accepted")
	}
}
