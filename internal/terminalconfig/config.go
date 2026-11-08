// Package terminalconfig contains presentation settings shared by SSH and Console.
package terminalconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const Path = "/etc/safeterm/terminal.json"

type Config struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

func (c Config) WithDefaults() Config {
	if c.Name == "" {
		c.Name = "safeterm"
	}
	if c.Prompt == "" {
		c.Prompt = "{brand}::{user}{context}{dirty}> "
	}
	return c
}

func (c Config) Title() string {
	c = c.WithDefaults()
	if c.Name == "safeterm" {
		return "SafeTerm"
	}
	return c.Name
}

func (c Config) Validate() error {
	c = c.WithDefaults()
	if len(c.Name) > 64 || len(c.Prompt) > 160 {
		return fmt.Errorf("terminal: name or prompt too long")
	}
	for _, value := range []string{c.Name, c.Prompt} {
		for _, r := range value {
			if r < 32 || r > 126 {
				return fmt.Errorf("terminal: use printable ASCII without terminal escape codes")
			}
		}
	}
	if strings.ContainsAny(c.Name, "{}") {
		return fmt.Errorf("terminal: name cannot contain braces")
	}
	for _, token := range []string{"{user}", "{context}", "{dirty}"} {
		if !strings.Contains(c.Prompt, token) {
			return fmt.Errorf("terminal: prompt must include %s", token)
		}
	}
	rest := strings.NewReplacer("{brand}", "", "{user}", "", "{context}", "", "{dirty}", "").Replace(c.Prompt)
	if strings.ContainsAny(rest, "{}") {
		return fmt.Errorf("terminal: unknown prompt placeholder")
	}
	return nil
}

func Load(path string) (Config, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Config{}.WithDefaults(), nil
	}
	if err != nil {
		return Config{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 4096 {
		return Config{}, fmt.Errorf("terminal: unsafe settings file %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var c Config
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("terminal: %w", err)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, fmt.Errorf("terminal: trailing JSON")
	}
	return c.WithDefaults(), c.Validate()
}
