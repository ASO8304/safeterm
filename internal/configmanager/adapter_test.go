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
