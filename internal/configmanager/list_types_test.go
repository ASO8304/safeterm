package configmanager

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestListValidationAndDraft(t *testing.T) {
	f := FieldSchema{Key: "PATIP", Type: FieldList, ItemType: FieldIPv4, Separator: ",", Required: true}
	if err := validateFields(Resource{ID: "list", Fields: []FieldSchema{f}}); err != nil {
		t.Fatal(err)
	}
	d := FieldDescription{Key: f.Key, Type: f.Type, ItemType: f.ItemType, Separator: f.Separator, Required: true}
	for _, value := range []string{"192.168.222.10", "192.168.222.10,10.20.30.40,129.45.21.11,129.39.33.22"} {
		typed, normalized, err := normalizeValue(f, value)
		if err != nil || typed != value || normalized != value {
			t.Fatalf("%q: %v", value, err)
		}
		if err := CheckDraftValue(d, value); err != nil {
			t.Fatal(err)
		}
		for _, format := range []Format{FormatINI, FormatJSON} {
			r := Resource{ID: "list", Format: format, Fields: []FieldSchema{f}}
			data := []byte("PATIP = 127.0.0.1\n")
			if format == FormatJSON {
				data = []byte(`{"PATIP":"127.0.0.1"}`)
			}
			rendered, _, _, _, err := renderPatch(r, data, Patch{Key: "PATIP", Value: value})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := parseDocument(format, rendered)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := doc.Get("", "PATIP")
			if !ok || scalarString(got) != value {
				t.Fatalf("round trip %s: %v", format, got)
			}
			if _, _, _, _, err := renderPatch(r, data, Patch{Key: "PATIP", Value: value + ","}); err == nil {
				t.Fatal("writer accepted trailing comma")
			}
		}
	}
	for _, value := range []string{"", "192.168.222.10,", ",192.168.222.10", "192.168.222.10,,10.20.30.40", "999.1.1.1", "192.168.222.10,::1", "192.168.222.10, 10.20.30.40", "192.168.222.10\n"} {
		if _, _, err := normalizeValue(f, value); err == nil {
			t.Errorf("accepted %q", value)
		}
		if err := CheckDraftValue(d, value); err == nil {
			t.Errorf("draft accepted %q", value)
		}
	}
}

func TestListSchemaAndBounds(t *testing.T) {
	one, two := 1, 2
	base := FieldSchema{Key: "PATIP", Type: FieldList, ItemType: FieldIPv4, Separator: ",", MinItems: &one, MaxItems: &two}
	if _, _, err := normalizeValue(base, "1.2.3.4,2.3.4.5,3.4.5.6"); err == nil {
		t.Fatal("ignored max_items")
	}
	base.MinItems = &two
	if _, _, err := normalizeValue(base, "1.2.3.4"); err == nil {
		t.Fatal("ignored min_items")
	}
	for _, mutate := range []func(*FieldSchema){
		func(f *FieldSchema) { f.ItemType = FieldList },
		func(f *FieldSchema) { f.ItemType = "unknown" },
		func(f *FieldSchema) { f.Separator = "" },
		func(f *FieldSchema) { f.Separator = ";" },
		func(f *FieldSchema) { f.Type = FieldIPv4 },
		func(f *FieldSchema) { f.MinItems = &two; f.MaxItems = &one },
	} {
		f := base
		mutate(&f)
		if err := validateFields(Resource{ID: "list", Fields: []FieldSchema{f}}); err == nil {
			t.Errorf("accepted %+v", f)
		}
	}
	base.AllowEmpty = true
	if _, _, err := normalizeValue(base, ""); err != nil {
		t.Fatal(err)
	}
}

func TestListItemsUseTypedValidation(t *testing.T) {
	for _, tc := range []struct {
		typ            FieldType
		valid, invalid string
	}{
		{FieldIPv4, "192.0.2.1,10.0.0.1", "192.0.2.1,256.0.0.1"},
		{FieldIPv6, "::1,2001:db8::1", "::1,192.0.2.1"},
		{FieldIP, "192.0.2.1,::1", "192.0.2.1,localhost"},
		{FieldCIDR, "192.0.2.1/24,2001:db8::/32", "192.0.2.1/24,192.0.2.2/33"},
		{FieldIPPort, "127.0.0.1:2420,[::1]:443", "127.0.0.1:2420,[::1]:65536"},
		{FieldPort, "80,443", "80,0"},
		{FieldInteger, "1,-2", "1,hello"},
		{FieldBoolean, "true,false", "true,yes"},
	} {
		t.Run(string(tc.typ), func(t *testing.T) {
			f := FieldSchema{Key: "Items", Type: FieldList, ItemType: tc.typ, Separator: ","}
			if _, _, err := normalizeValue(f, tc.valid); err != nil {
				t.Fatal(err)
			}
			if _, _, err := normalizeValue(f, tc.invalid); err == nil || !strings.Contains(err.Error(), "list item 2") {
				t.Fatalf("missing item diagnostic: %v", err)
			}
		})
	}
}

func TestListManagerDescribePreviewApplyPreservesINI(t *testing.T) {
	path := writeTestINI(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const old = "192.168.222.10"
	data = []byte(strings.Replace(string(data), `Cat_Port = "99"`, `Cat_Port = "`+old+`" ; keep this comment`, 1))
	if err := os.WriteFile(path, data, 0640); err != nil {
		t.Fatal(err)
	}
	reg := testRegistry(path, false)
	one, four := 1, 4
	reg.Resources[0].Fields[1] = FieldSchema{Section: "cats", Key: "Cat_Port", Type: FieldList, ItemType: FieldIPv4, Separator: ",", MinItems: &one, MaxItems: &four, Required: true}
	m, err := NewManager("application", reg, nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := actor("alice", "viewer", "operator")
	described, err := m.Describe(context.Background(), a, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	f := described.Fields[1]
	if f.Type != FieldList || f.ItemType != FieldIPv4 || f.Separator != "," || f.MaxItems == nil || *f.MaxItems != 4 {
		t.Fatalf("lost list metadata: %+v", f)
	}
	value := old + ",10.20.30.40,129.45.21.11,129.39.33.22"
	if err := CheckDraftValue(f, value); err != nil {
		t.Fatal(err)
	}
	read, err := m.Read(context.Background(), a, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	patch := Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: old + ",999.1.1.1", BaseVersion: read.Version}
	if _, err := m.Preview(context.Background(), a, patch); err == nil || !strings.Contains(err.Error(), "list item 2") {
		t.Fatalf("invalid preview accepted: %v", err)
	}
	patch.Value = value
	preview, err := m.Preview(context.Background(), a, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), a, preview.Token); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != strings.Replace(string(data), old, value, 1) {
		t.Fatalf("unrelated INI bytes changed: %q", got)
	}
}
