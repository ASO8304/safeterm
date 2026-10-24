package configmanager

import (
	"strings"
	"testing"
)

func TestNetworkFieldTypes(t *testing.T) {
	for _, tc := range []struct {
		typ            FieldType
		valid, invalid []string
	}{
		{FieldIPv4, []string{"192.0.2.1", "0.0.0.0", "255.255.255.255"}, []string{"256.1.1.1", "192.168.001.1", "::1", "192.0.2.1/24", "localhost", "any", "", " 192.0.2.1", "192.0.2.1 ", "192.0.2.1:80"}},
		{FieldIPv6, []string{"::1", "::", "2001:db8::1", "2001:0DB8:0000:0000:0000:0000:0000:0001"}, []string{"192.0.2.1", "::ffff:192.0.2.1", "fe80::1%eth0", "[::1]", "2001:db8::/64", "any", "", "gggg::1"}},
		{FieldIP, []string{"192.0.2.1", "2001:db8::1"}, []string{"any", "localhost", "999.1.1.1", "::ffff:192.0.2.1", "fe80::1%eth0", "127.0.0.1\n", "127.0.0.1;id"}},
		{FieldCIDR, []string{"192.0.2.0/24", "192.0.2.7/24", "0.0.0.0/0", "2001:db8::1/64", "::/0", "::1/128"}, []string{"192.0.2.1", "192.0.2.1/33", "2001:db8::/129", "::ffff:192.0.2.1/120", "fe80::%eth0/64", "any", "", "192.0.2.1/-1", "192.0.2.1/255.255.255.0"}},
	} {
		t.Run(string(tc.typ), func(t *testing.T) {
			field := FieldSchema{Section: "Network", Key: "Address", Type: tc.typ}
			if err := validateFields(Resource{ID: "network", Fields: []FieldSchema{field}}); err != nil {
				t.Fatal(err)
			}
			for _, value := range tc.valid {
				typed, normalized, err := normalizeValue(field, value)
				if err != nil || typed != value || normalized != value {
					t.Errorf("%q: (%v,%q,%v)", value, typed, normalized, err)
				}
				if err := CheckDraftValue(FieldDescription{Section: field.Section, Key: field.Key, Type: field.Type}, value); err != nil {
					t.Fatal(err)
				}
			}
			for _, value := range tc.invalid {
				if _, _, err := normalizeValue(field, value); err == nil {
					t.Errorf("accepted %q", value)
				}
				if err := CheckDraftValue(FieldDescription{Type: field.Type}, value); err == nil {
					t.Errorf("draft accepted %q", value)
				}
			}
		})
	}
}

func TestNetworkFieldsRespectConstraintsAndRejectIncompatibleSchema(t *testing.T) {
	for _, typ := range []FieldType{FieldIPv4, FieldIPv6, FieldIP, FieldCIDR} {
		for _, field := range []FieldSchema{
			{Type: typ, Min: int64Pointer(1)}, {Type: typ, Max: int64Pointer(1)}, {Type: typ, Enum: []string{"192.0.2.1"}}, {Type: typ, Path: &PathPolicy{AllowedPrefixes: []string{"/etc"}}},
		} {
			field.Key = "Address"
			if err := validateFields(Resource{ID: "network", Fields: []FieldSchema{field}}); err == nil {
				t.Errorf("accepted invalid schema %+v", field)
			}
		}
	}
	f := FieldSchema{Key: "Address", Type: FieldIPv4, Pattern: `^10\.`}
	if _, _, err := normalizeValue(f, "192.0.2.1"); err == nil {
		t.Fatal("ignored extra pattern")
	}
	if _, _, err := normalizeValue(f, "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkFieldPatchPreservesINIAndJSONStrings(t *testing.T) {
	for _, format := range []Format{FormatINI, FormatJSON} {
		for _, tc := range []struct {
			typ      FieldType
			old, new string
		}{
			{FieldIPv4, "192.0.2.1", "192.0.2.2"}, {FieldIPv6, "::1", "2001:0DB8::1"}, {FieldIP, "192.0.2.1", "2001:db8::1"}, {FieldCIDR, "192.0.2.1/24", "192.0.2.7/24"},
		} {
			var data string
			if format == FormatINI {
				data = "\ufeff# comment\r\n[Network]\r\n\r\nAddress = " + tc.old + " ; keep\r\nUnmanaged = untouched\r\n"
			} else {
				data = `{"Network":{"Address":"` + tc.old + `","Unmanaged":"untouched"}}`
			}
			r := Resource{ID: "network", Format: format, Fields: []FieldSchema{{Section: "Network", Key: "Address", Type: tc.typ}}}
			rendered, _, _, _, err := renderPatch(r, []byte(data), Patch{Section: "Network", Key: "Address", Value: tc.new})
			if err != nil {
				t.Fatal(err)
			}
			if format == FormatINI && string(rendered) != strings.Replace(data, tc.old, tc.new, 1) {
				t.Fatalf("INI layout changed: %q", rendered)
			}
			doc, err := parseDocument(format, rendered)
			if err != nil {
				t.Fatal(err)
			}
			v, _ := doc.Get("Network", "Address")
			if v != tc.new {
				t.Fatalf("value=%v", v)
			}
			if _, _, _, _, err := renderPatch(r, []byte(data), Patch{Section: "Network", Key: "Address", Value: "any"}); err == nil {
				t.Fatal("writer path accepted any")
			}
		}
	}
}
