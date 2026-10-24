package configmanager

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAllowEmptySeparatesMissingFromBlank(t *testing.T) {
	for _, required := range []bool{false, true} {
		for _, allow := range []bool{false, true} {
			f := FieldSchema{Section: "Sultan", Key: "PATIP", Type: FieldIPv4, Required: required, AllowEmpty: allow}
			r := Resource{ID: "sultan", Path: "/etc/sultan/option.ini", Format: FormatINI, Fields: []FieldSchema{f}}
			for _, tc := range []struct {
				name, content string
				wantOK        bool
			}{
				{"missing", "[Sultan]\nOther = unchanged\n", !required},
				{"empty", "[Sultan]\nPATIP = \n", allow},
				{"valid", "[Sultan]\nPATIP = 192.0.2.1\n", true},
				{"invalid", "[Sultan]\nPATIP = any\n", false},
			} {
				doc, err := parseDocument(FormatINI, []byte(tc.content))
				if err != nil {
					t.Fatal(err)
				}
				err = validateCompleteDocument(r, doc)
				if (err == nil) != tc.wantOK {
					t.Errorf("required=%t allow_empty=%t %s: %v", required, allow, tc.name, err)
				}
				if err != nil {
					for _, part := range []string{`resource "sultan"`, `file "/etc/sultan/option.ini"`, `section "Sultan"`, `key "PATIP"`, "type=ipv4", "required=", "allow_empty="} {
						if !strings.Contains(err.Error(), part) {
							t.Errorf("missing context %s in %v", part, err)
						}
					}
				}
			}
		}
	}
}

func TestAllowEmptyDraftAndTypedPatch(t *testing.T) {
	for _, typ := range []FieldType{FieldString, FieldInteger, FieldPort, FieldBoolean, FieldIPv4, FieldIPv6, FieldIP, FieldCIDR, FieldIPPort, FieldEnum, FieldPath} {
		f := FieldSchema{Section: "Sultan", Key: "PATIP", Type: typ, Required: true, AllowEmpty: true}
		if typ == FieldEnum {
			f.Enum = []string{"enabled"}
		}
		if typ == FieldPath {
			f.Path = &PathPolicy{AllowedPrefixes: []string{"/etc"}, MustExist: true}
		}
		if err := validateFields(Resource{ID: "sultan", Fields: []FieldSchema{f}}); err != nil {
			t.Fatal(err)
		}
		if err := CheckDraftValue(FieldDescription{Type: typ, Required: true, AllowEmpty: true, Enum: f.Enum}, ""); err != nil {
			t.Fatalf("draft type %s: %v", typ, err)
		}
		for _, format := range []Format{FormatINI, FormatJSON} {
			data := []byte("\ufeff# comment\r\n[Sultan]\r\nPATIP =  ; keep\r\nOther = unchanged\r\n")
			if format == FormatJSON {
				data = []byte(`{"Sultan":{"PATIP":"","Other":"unchanged"}}`)
			}
			r := Resource{ID: "sultan", Path: "/etc/sultan/option.ini", Format: format, Fields: []FieldSchema{f}}
			out, _, _, _, err := renderPatch(r, data, Patch{Section: "Sultan", Key: "PATIP", Value: ""})
			if err != nil {
				t.Fatalf("%s %s: %v", typ, format, err)
			}
			if format == FormatINI && string(out) != string(data) {
				t.Fatalf("blank edit changed INI: %q", out)
			}
			doc, err := parseDocument(format, out)
			if err != nil {
				t.Fatal(err)
			}
			value, exists := doc.Get("Sultan", "PATIP")
			if !exists || value != "" {
				t.Fatalf("blank not retained: %v", value)
			}
		}
	}
}

func TestAllowEmptyErrorsDoNotRevealSecrets(t *testing.T) {
	const secret = "private-path-do-not-log"
	f := FieldSchema{Section: "Sultan", Key: "PATIP", Type: FieldPath, Secret: true, Path: &PathPolicy{AllowedPrefixes: []string{"/etc"}, MustExist: true}}
	r := Resource{ID: "sultan", Path: "/etc/sultan/option.ini", Fields: []FieldSchema{f}}
	doc, err := parseDocument(FormatINI, []byte("[Sultan]\nPATIP = /etc/"+secret+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	err = validateCompleteDocument(r, doc)
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "redacted") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestManagerDescribesAndAppliesAllowEmpty(t *testing.T) {
	path := writeTestINI(t)
	registry := testRegistry(path, false)
	for i := range registry.Resources[0].Fields {
		if registry.Resources[0].Fields[i].Key == "Cat_Port" {
			registry.Resources[0].Fields[i].AllowEmpty = true
		}
	}
	m, err := NewManager("application", registry, nil, nil, &recordingSink{}, ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := actor("alice", "viewer", "operator")
	description, err := m.Describe(context.Background(), a, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range description.Fields {
		if f.Key == "Cat_Port" {
			found = f.AllowEmpty
		}
	}
	if !found {
		t.Fatal("allow_empty lost in description")
	}
	read, err := m.Read(context.Background(), a, "cats-options")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := m.Preview(context.Background(), a, Patch{ResourceID: "cats-options", Section: "cats", Key: "Cat_Port", Value: "", BaseVersion: read.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), a, preview.Token); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parseDocument(FormatINI, data)
	if err != nil {
		t.Fatal(err)
	}
	v, exists := doc.Get("cats", "Cat_Port")
	if !exists || v != "" {
		t.Fatalf("blank value not saved: %v", v)
	}
}
