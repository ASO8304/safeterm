package broker

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"safeterm/internal/configmanager"
)

const dirtyDraftReminder = "[!] Unsaved edits exist. Use 'next' to stage them or 'save' to commit."
const exitDescription = "Move up one level; asks before discarding unsaved edits"
const nextDescription = "Move up (keeps unsaved edits staged)"

var globalDescriptions = map[string]string{
	"exit":   "Close session; asks before discarding unsaved drafts",
	"clear":  "Clear the screen",
	"whoami": "Show your identity and roles",
}

// Globals are resolved before editors, including help, so exit ? cannot exit.
func (s *Shell) globalCommand(ctx context.Context, fields []string) (handled, exit bool, err error) {
	command := fields[0]
	if command == "logout" || command == "quit" {
		command = "exit"
	}
	if command == "whoami" && !s.atRoot() {
		return true, false, fmt.Errorf("whoami is available only at the main prompt; use exit to move up")
	}
	description, ok := globalDescriptions[command]
	if !ok {
		return false, false, nil
	}
	if command == "exit" && !s.atRoot() {
		description = exitDescription
	}
	if len(fields) == 2 && fields[1] == "?" {
		s.networkHelpRow("<cr>", description)
		return true, false, nil
	}
	if len(fields) != 1 {
		return true, false, fmt.Errorf("usage: %s", command)
	}
	switch command {
	case "exit":
		if s.networkEdit != nil {
			return true, false, s.networkEditor(ctx, "exit")
		}
		if s.editing != nil {
			_, err := s.configEditor(ctx, "exit", nil)
			return true, false, err
		}
		if s.configMenu != nil {
			path := append([]string{}, s.configMenu...)
			s.discardLogicalUnder(path)
			if len(path) == 0 {
				s.configMenu = nil
				return true, false, nil
			}
			return true, false, s.openConfigurationPath(ctx, path[:len(path)-1], false)
		}
		if s.editorDirty() || len(s.stagedLogical) > 0 {
			s.writeStyled(styleWarning, "Warning: Unsaved drafts were discarded before closing the session.\n")
		}
		s.editing, s.networkEdit, s.configMenu, s.stagedLogical = nil, nil, nil, nil
		return true, true, nil
	case "clear":
		fmt.Fprint(s.output, "\x1b[2J\x1b[H")
	case "whoami":
		s.writeTerminalData([]byte(fmt.Sprintf("user: %s\nroles: %s\n", s.principal.UserID, strings.Join(s.principal.Roles, ", "))))
	}
	return true, false, nil
}

func (s *Shell) editorDirty() bool {
	if e := s.editing; e != nil {
		return len(e.changes) > 0
	}
	if e := s.networkEdit; e != nil {
		return len(networkDiffs(e)) > 0
	}
	return len(s.stagedLogical) > 0
}

// Remove fields restored to their initial value. Secret values remain dirty:
// the initial secret is deliberately unavailable to the terminal.
func (e *configEditSession) pruneChanges() {
	e.changes = slices.DeleteFunc(e.changes, func(c configmanager.FieldChange) bool {
		for _, f := range e.base.Fields {
			if f.Section == c.Section && f.Key == c.Key {
				return !f.Redacted && f.Value == c.Value
			}
		}
		return false
	})
}

func (s *Shell) discardedItem(name string) {
	s.writeStyled(styleWarning, fmt.Sprintf("Discarded unsaved edits for '%s'.\n", name))
}

func (s *Shell) discardLogicalUnder(path []string) {
	keys := make([]string, 0, len(s.stagedLogical))
	for key := range s.stagedLogical {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if e := s.stagedLogical[key]; wordsStart(e.path, path) {
			s.discardedItem(e.path[len(e.path)-1])
			delete(s.stagedLogical, key)
		}
	}
}

func (s *Shell) atRoot() bool { return s.editing == nil && s.networkEdit == nil && s.configMenu == nil }
