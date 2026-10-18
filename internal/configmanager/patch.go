package configmanager

import (
	"fmt"
	"strings"
)

// MaxPatchChanges bounds both interactive drafts and IPC resource updates.
const MaxPatchChanges = 512

func patchChanges(p Patch) ([]FieldChange, error) {
	if len(p.Changes) == 0 {
		if p.Key == "" {
			return nil, fmt.Errorf("configmanager: no field changes")
		}
		return []FieldChange{{Section: p.Section, Key: p.Key, Value: p.Value}}, nil
	}
	if p.Section != "" || p.Key != "" || p.Value != "" || len(p.Changes) > MaxPatchChanges {
		return nil, fmt.Errorf("configmanager: invalid mixed or excessive field changes")
	}
	seen := map[[2]string]bool{}
	for _, c := range p.Changes {
		id := [2]string{c.Section, c.Key}
		if c.Key == "" || seen[id] {
			return nil, fmt.Errorf("configmanager: duplicate or empty field")
		}
		seen[id] = true
	}
	return p.Changes, nil
}

func patchCoordinate(p Patch) string {
	if len(p.Changes) != 0 {
		return fmt.Sprintf("batch:%d", len(p.Changes))
	}
	return fieldCoordinate(p.Section, p.Key)
}

// CheckDraftValue gives immediate scalar feedback. Full path, cross-field and
// application validation remains authoritative in Preview/Apply and the writer.
func CheckDraftValue(f FieldDescription, value string) error {
	typ := f.Type
	if typ == FieldPath {
		typ = FieldString
	}
	_, _, err := normalizeValue(FieldSchema{Section: f.Section, Key: f.Key, Type: typ, Required: f.Required, Enum: f.Enum, Min: f.Min, Max: f.Max, MinLength: f.MinLength, MaxLength: f.MaxLength, Pattern: f.Pattern}, value)
	return err
}

// renderPatch is shared by the manager and privileged writer. Neither accepts
// arbitrary replacement documents; every change is independently registered.
func renderPatch(r Resource, data []byte, p Patch) ([]byte, string, string, bool, error) {
	changes, err := patchChanges(p)
	if err != nil {
		return nil, "", "", false, err
	}
	doc, err := parseDocument(r.Format, data)
	if err != nil {
		return nil, "", "", false, err
	}
	diffs := []string{}
	normalized := make([]string, len(changes))
	secret := false
	for i, c := range changes {
		f, ok := findField(r, c.Section, c.Key)
		if !ok || f.ReadOnly {
			return nil, "", "", false, fmt.Errorf("configmanager: field is unknown or read-only")
		}
		old, exists := doc.Get(c.Section, c.Key)
		if !exists {
			return nil, "", "", false, fmt.Errorf("configmanager: registered field is missing from document")
		}
		value, norm, e := normalizeValue(f, c.Value)
		if e != nil {
			return nil, "", "", false, e
		}
		normalized[i] = norm
		if r.Format == FormatJSON {
			value = typedValueForJSON(value)
		}
		if e = doc.Set(c.Section, c.Key, value); e != nil {
			return nil, "", "", false, e
		}
		diffs = append(diffs, renderDiff(f, scalarString(old), norm))
		secret = secret || f.Secret
	}
	if err = validateCompleteDocument(r, doc); err != nil {
		return nil, "", "", false, err
	}
	rendered, err := doc.Render()
	if err != nil {
		return nil, "", "", false, err
	}
	if len(rendered) > maxDocumentBytes {
		return nil, "", "", false, fmt.Errorf("configmanager: rendered document exceeds size limit")
	}
	reparsed, err := parseDocument(r.Format, rendered)
	if err != nil {
		return nil, "", "", false, err
	}
	if err = validateCompleteDocument(r, reparsed); err != nil {
		return nil, "", "", false, err
	}
	for i, c := range changes {
		actual, exists := reparsed.Get(c.Section, c.Key)
		if !exists || scalarString(actual) != normalized[i] {
			return nil, "", "", false, fmt.Errorf("configmanager: value changed during rendering")
		}
	}
	return rendered, strings.Join(diffs, "\n"), strings.Join(normalized, "\n"), secret, nil
}
