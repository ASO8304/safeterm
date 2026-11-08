package terminalconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresentationDefaultsAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.json")
	c, err := Load(path)
	if err != nil || c.Name != "safeterm" || c.Prompt != "{brand}::{user}{context}{dirty}> " {
		t.Fatal(c, err)
	}
	for _, data := range []string{`{"name":"Acme","prompt":"{brand}/{user}{context}{dirty}$ "}`, `{}`} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, data := range []string{`{"name":"\u001b[2J"}`, `{"prompt":"{user}> "}`, `{"prompt":"{user}{context}{dirty}{unknown}"}`, `{"unexpected":true}`, `{} {}`} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("invalid presentation accepted", data)
		}
	}
}
