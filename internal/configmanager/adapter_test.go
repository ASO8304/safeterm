package configmanager

import (
	"strings"
	"testing"
)

func TestINIPreservesLayoutCommentsQuotesAndCRLF(t *testing.T) {
	original := []byte("; keep this\r\n[cats]\r\nCat_Path = \"/etc/cat\"\r\nCat_Port = '99' ; inline\r\n")
	document, err := parseDocument(FormatINI, original)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Set("cats", "Cat_Port", int64(100)); err != nil {
		t.Fatal(err)
	}
	rendered, err := document.Render()
	if err != nil {
		t.Fatal(err)
	}
	value := string(rendered)
	for _, expected := range []string{"; keep this\r\n", "Cat_Path = \"/etc/cat\"", "Cat_Port = '100' ; inline"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("rendered INI lost %q:\n%s", expected, value)
		}
	}
}

func TestJSONRenderIsDeterministic(t *testing.T) {
	document, err := parseDocument(FormatJSON, []byte(`{"z":1,"server":{"port":99,"enabled":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Set("server", "port", int64(100)); err != nil {
		t.Fatal(err)
	}
	first, _ := document.Render()
	second, _ := document.Render()
	if string(first) != string(second) || !strings.HasSuffix(string(first), "\n") {
		t.Fatalf("non-deterministic JSON: %q / %q", first, second)
	}
}

func TestINIRejectsDuplicateFields(t *testing.T) {
	if _, err := parseDocument(FormatINI, []byte("[x]\na=1\na=2\n")); err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestJSONRejectsDuplicateFields(t *testing.T) {
	if _, err := parseDocument(FormatJSON, []byte(`{"server":{"port":99,"port":100}}`)); err == nil {
		t.Fatal("expected duplicate JSON key rejection")
	}
}

func TestJSONRejectsExcessiveNesting(t *testing.T) {
	if _, err := parseJSON([]byte(`{"nested":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`)); err == nil {
		t.Fatal("unbounded nesting accepted")
	}
}

func TestINIUTF8BOMRoundTripAndEditPreserveAllOtherBytes(t *testing.T) {
	for _, bom := range []string{"", "\xef\xbb\xbf"} {
		for _, ending := range []string{"\n", "\r\n"} {
			for _, header := range []string{"", "# comment" + ending, ending + "; comment" + ending + ending} {
				for _, final := range []string{"", ending} {
					original := bom + header + strings.Join([]string{"[Cat]", "", "# Remote configuration", "AllowRemoteConfiguration = False", "", "; Keep timeout", "KeepAwake_Timeout = 90", "", "KeepAwake_Interval = 15"}, ending) + final
					doc, err := parseDocument(FormatINI, []byte(original))
					if err != nil {
						t.Fatal(err)
					}
					rendered, err := doc.Render()
					if err != nil || string(rendered) != original {
						t.Fatalf("round trip changed original: %q %v", rendered, err)
					}
					if value, ok := doc.Get("Cat", "KeepAwake_Timeout"); !ok || value != "90" {
						t.Fatalf("wrong parsed value: %v %v", value, ok)
					}
					if err = doc.Set("Cat", "KeepAwake_Timeout", int64(120)); err != nil {
						t.Fatal(err)
					}
					rendered, err = doc.Render()
					want := strings.Replace(original, "KeepAwake_Timeout = 90", "KeepAwake_Timeout = 120", 1)
					if err != nil || string(rendered) != want {
						t.Fatalf("edit changed unrelated bytes: %q %v", rendered, err)
					}
					if _, err = parseDocument(FormatINI, rendered); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestINIUTF8BOMBeforeRootKeyIsPreservedOnEdit(t *testing.T) {
	original := "\xef\xbb\xbfPort = 8080 # keep\r\n"
	doc, err := parseINI([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Set("", "Port", int64(9090)); err != nil {
		t.Fatal(err)
	}
	rendered, _ := doc.Render()
	if string(rendered) != strings.Replace(original, "8080", "9090", 1) {
		t.Fatalf("lost root-key BOM: %q", rendered)
	}
}

func TestINIBOMDoesNotHideMalformedOrDuplicateSyntax(t *testing.T) {
	for _, data := range []string{"\xef\xbb\xbf\xef\xbb\xbf[Cat]\nA=1\n", "[Cat]\n\xef\xbb\xbfA=1\n", "\xef\xbb\xbf[Cat]\nA=1\nA=2\n", "\xef\xbb\xbfNot an assignment\n", "\xff\xfe[Cat]\x00"} {
		if _, err := parseINI([]byte(data)); err == nil {
			t.Fatalf("accepted invalid syntax: %q", data)
		}
	}
	for _, data := range []string{"\xef\xbb\xbf", "\xef\xbb\xbf\n"} {
		doc, err := parseINI([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		rendered, _ := doc.Render()
		if string(rendered) != data {
			t.Fatal("BOM-only round trip changed bytes")
		}
	}
}
