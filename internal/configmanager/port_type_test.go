package configmanager

import (
	"strings"
	"testing"
)

func TestPortTypeBoundsAndDraftValidation(t *testing.T) {
	f := FieldSchema{Section: "Server", Key: "Port", Type: FieldPort}
	if err := validateFields(Resource{ID: "server", Fields: []FieldSchema{f}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input string
		want  int64
	}{{"1", 1}, {"443", 443}, {"65535", 65535}} {
		value, normalized, err := normalizeValue(f, tc.input)
		if err != nil || value != tc.want || normalized != tc.input {
			t.Fatalf("%s: %v %q %v", tc.input, value, normalized, err)
		}
		if err := CheckDraftValue(FieldDescription{Type: FieldPort}, tc.input); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"0", "-1", "65536", "9999999999999999999999999999", "1.5", "http", "any", ""} {
		if _, _, err := normalizeValue(f, input); err == nil {
			t.Fatalf("accepted port %q", input)
		}
		if err := CheckDraftValue(FieldDescription{Type: FieldPort}, input); err == nil {
			t.Fatalf("draft accepted %q", input)
		}
	}
	f.Min = int64Pointer(1024)
	f.Max = int64Pointer(49151)
	for _, input := range []string{"443", "50000"} {
		if _, _, err := normalizeValue(f, input); err == nil {
			t.Fatalf("ignored narrower range %s", input)
		}
	}
	for _, bound := range []int64{0, -1, 65536} {
		for _, f := range []FieldSchema{{Key: "Port", Type: FieldPort, Min: int64Pointer(bound)}, {Key: "Port", Type: FieldPort, Max: int64Pointer(bound)}} {
			if err := validateFields(Resource{ID: "server", Fields: []FieldSchema{f}}); err == nil {
				t.Fatalf("accepted wider port bounds %+v", f)
			}
		}
	}
}

func TestPortPatchRendersNumericJSONAndPreservesINI(t *testing.T) {
	for _, format := range []Format{FormatINI, FormatJSON} {
		data := []byte("# preserve\n[Server]\nPort = 80 ; note\n")
		if format == FormatJSON {
			data = []byte(`{"Server":{"Port":80}}`)
		}
		r := Resource{ID: "server", Format: format, Fields: []FieldSchema{{Section: "Server", Key: "Port", Type: FieldPort}}}
		out, _, _, _, err := renderPatch(r, data, Patch{Section: "Server", Key: "Port", Value: "443"})
		if err != nil {
			t.Fatal(err)
		}
		if format == FormatINI && string(out) != strings.Replace(string(data), "80", "443", 1) {
			t.Fatalf("format changed: %s", out)
		}
		if format == FormatJSON && (!strings.Contains(string(out), `"Port": 443`) || strings.Contains(string(out), `"443"`)) {
			t.Fatalf("JSON port must be a number: %s", out)
		}
		for _, input := range []string{"0", "65536"} {
			if _, _, _, _, err := renderPatch(r, data, Patch{Section: "Server", Key: "Port", Value: input}); err == nil {
				t.Fatalf("writer path accepted %s", input)
			}
		}
	}
}
