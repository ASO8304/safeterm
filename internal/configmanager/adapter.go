package configmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxDocumentBytes = 4 << 20

type document interface {
	Get(section, key string) (any, bool)
	Set(section, key string, value any) error
	Render() ([]byte, error)
}

func parseDocument(format Format, data []byte) (document, error) {
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("configmanager: document exceeds %d bytes", maxDocumentBytes)
	}
	switch format {
	case FormatINI:
		return parseINI(data)
	case FormatJSON:
		return parseJSON(data)
	default:
		return nil, fmt.Errorf("configmanager: unsupported format %q", format)
	}
}

type iniValue struct {
	line   int
	prefix string
	suffix string
	quote  byte
	value  string
}

type iniDocument struct {
	bom    string
	lines  []string
	values map[string]*iniValue
}

func parseINI(data []byte) (*iniDocument, error) {
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("configmanager: INI contains NUL")
	}
	text := string(data)
	// A UTF-8 signature is file metadata, not part of the first section/key.
	// Keep it separately so round trips and edits preserve the original bytes.
	bom := ""
	if strings.HasPrefix(text, "\xef\xbb\xbf") {
		bom, text = text[:3], text[3:]
	}
	lines := splitLinesPreservingEndings(text)
	document := &iniDocument{bom: bom, lines: lines, values: map[string]*iniValue{}}
	section := ""
	sections := map[string]bool{}
	for index, line := range lines {
		body, ending := trimLineEnding(line)
		trimmed := strings.TrimSpace(body)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			end := strings.IndexByte(trimmed, ']')
			if end < 1 {
				return nil, fmt.Errorf("configmanager: invalid INI section on line %d", index+1)
			}
			trailing := strings.TrimSpace(trimmed[end+1:])
			if trailing != "" && !strings.HasPrefix(trailing, ";") && !strings.HasPrefix(trailing, "#") {
				return nil, fmt.Errorf("configmanager: invalid INI section on line %d", index+1)
			}
			section = strings.TrimSpace(trimmed[1:end])
			if !fieldIdentifierPattern.MatchString(section) || sections[section] {
				return nil, fmt.Errorf("configmanager: invalid or duplicate INI section on line %d", index+1)
			}
			sections[section] = true
			continue
		}
		equals := strings.IndexByte(body, '=')
		if equals < 1 {
			return nil, fmt.Errorf("configmanager: invalid INI assignment on line %d", index+1)
		}
		key := strings.TrimSpace(body[:equals])
		if !fieldIdentifierPattern.MatchString(key) {
			return nil, fmt.Errorf("configmanager: invalid INI key on line %d", index+1)
		}
		after := body[equals+1:]
		leadingCount := len(after) - len(strings.TrimLeft(after, " \t"))
		valueEnd := iniValueEnd(after, leadingCount)
		rawEnd := valueEnd
		for rawEnd > leadingCount && (after[rawEnd-1] == ' ' || after[rawEnd-1] == '\t') {
			rawEnd--
		}
		rawValue := after[leadingCount:rawEnd]
		value, quote, err := decodeINIValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("configmanager: invalid INI value on line %d: %w", index+1, err)
		}
		coordinate := section + "\x00" + key
		if _, exists := document.values[coordinate]; exists {
			return nil, fmt.Errorf("configmanager: duplicate INI key %q.%q", section, key)
		}
		document.values[coordinate] = &iniValue{
			line: index, prefix: body[:equals+1] + after[:leadingCount],
			suffix: after[rawEnd:] + ending, quote: quote, value: value,
		}
	}
	return document, nil
}

func iniValueEnd(value string, start int) int {
	var quote byte
	for index := start; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote && (quote == '\'' || index == 0 || value[index-1] != '\\') {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if (character == ';' || character == '#') && index > start && (value[index-1] == ' ' || value[index-1] == '\t') {
			return index
		}
	}
	return len(value)
}

func splitLinesPreservingEndings(text string) []string {
	if text == "" {
		return []string{}
	}
	result := []string{}
	for len(text) > 0 {
		index := strings.IndexByte(text, '\n')
		if index < 0 {
			result = append(result, text)
			break
		}
		result = append(result, text[:index+1])
		text = text[index+1:]
	}
	return result
}

func trimLineEnding(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}

func decodeINIValue(raw string) (string, byte, error) {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		value, err := strconv.Unquote(raw)
		return value, '"', err
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		if strings.Contains(raw[1:len(raw)-1], "'") {
			return "", 0, fmt.Errorf("single-quoted values cannot contain a quote")
		}
		return raw[1 : len(raw)-1], '\'', nil
	}
	if strings.ContainsAny(raw, "\"'") {
		return "", 0, fmt.Errorf("unbalanced quote")
	}
	return raw, 0, nil
}

func (d *iniDocument) Get(section, key string) (any, bool) {
	value, ok := d.values[section+"\x00"+key]
	if !ok {
		return nil, false
	}
	return value.value, true
}

func (d *iniDocument) Set(section, key string, value any) error {
	entry, ok := d.values[section+"\x00"+key]
	if !ok {
		return fmt.Errorf("configmanager: INI field %q.%q does not exist", section, key)
	}
	encoded := scalarString(value)
	if strings.ContainsAny(encoded, "\r\n\x00") {
		return fmt.Errorf("configmanager: multiline INI values are not supported")
	}
	switch entry.quote {
	case '"':
		encoded = strconv.Quote(encoded)
	case '\'':
		if strings.Contains(encoded, "'") {
			return fmt.Errorf("configmanager: value cannot be represented with existing INI quoting")
		}
		encoded = "'" + encoded + "'"
	}
	if entry.quote == 0 {
		if strings.TrimSpace(encoded) != encoded || strings.ContainsAny(encoded, "\"'") || iniValueEnd(encoded, 0) != len(encoded) {
			return fmt.Errorf("configmanager: value cannot be represented with existing INI quoting")
		}
	}
	d.lines[entry.line] = entry.prefix + encoded + entry.suffix
	entry.value = scalarString(value)
	return nil
}

func (d *iniDocument) Render() ([]byte, error) {
	return []byte(d.bom + strings.Join(d.lines, "")), nil
}

type jsonDocument struct {
	root map[string]any
}

func parseJSON(data []byte) (*jsonDocument, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder, 0)
	if err != nil {
		return nil, fmt.Errorf("configmanager: parse JSON: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return nil, fmt.Errorf("configmanager: JSON has trailing data")
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("configmanager: JSON root must be an object")
	}
	if root == nil {
		return nil, fmt.Errorf("configmanager: JSON root must be an object")
	}
	return &jsonDocument{root: root}, nil
}

func decodeJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("configmanager: JSON nesting exceeds 64 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate object key %q", key)
			}
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, fmt.Errorf("unterminated object")
		}
		return object, nil
	case '[':
		array := []any{}
		for decoder.More() {
			value, err := decodeJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, fmt.Errorf("unterminated array")
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected delimiter %q", delimiter)
	}
}

func (d *jsonDocument) section(name string) (map[string]any, bool) {
	if name == "" {
		return d.root, true
	}
	value, ok := d.root[name]
	if !ok {
		return nil, false
	}
	section, ok := value.(map[string]any)
	return section, ok
}

func (d *jsonDocument) Get(section, key string) (any, bool) {
	object, ok := d.section(section)
	if !ok {
		return nil, false
	}
	value, ok := object[key]
	if !ok || value == nil {
		return nil, ok
	}
	switch value.(type) {
	case string, bool, json.Number, float64:
		return value, true
	default:
		return nil, false
	}
}

func (d *jsonDocument) Set(section, key string, value any) error {
	object, ok := d.section(section)
	if !ok {
		return fmt.Errorf("configmanager: JSON section %q does not exist", section)
	}
	if _, exists := object[key]; !exists {
		return fmt.Errorf("configmanager: JSON field %q.%q does not exist", section, key)
	}
	object[key] = value
	return nil
}

func (d *jsonDocument) Render() ([]byte, error) {
	data, err := json.MarshalIndent(d.root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("configmanager: render JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}
