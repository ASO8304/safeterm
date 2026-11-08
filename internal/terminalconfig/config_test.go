package terminalconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresentationDefaultsAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.json")
	c, err := Load(path)
	if err != nil || c.Name != "safeterm" || c.Prompt != "{brand}::{user}{context}{dirty}> " || c.ShowResult {
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
	for _, enabled := range []bool{false, true} {
		data := `{"show_result":false}`
		if enabled {
			data = `{"show_result":true}`
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err != nil || c.ShowResult != enabled {
			t.Fatal(c, err)
		}
	}
	for _, data := range []string{`{"name":"\u001b[2J"}`, `{"prompt":"{user}> "}`, `{"prompt":"{user}{context}{dirty}{unknown}"}`, `{"unexpected":true}`, `{} {}`, `{"show_result":"false"}`, `{"show_result":1}`} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("invalid presentation accepted", data)
		}
	}
}
