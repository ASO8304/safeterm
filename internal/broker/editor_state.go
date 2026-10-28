package broker

import (
	"fmt"
	"slices"
	"strings"

	"safeterm/internal/configmanager"
)

const dirtyDraftReminder = "[!] Unsaved edits exist. Use 'next' to stage them or 'save' to commit."
const backDescription = "Move up (discards unsaved edits to this item)"
const nextDescription = "Move up (keeps unsaved edits staged)"

var globalDescriptions = map[string]string{
	"exit":   "Close session; discard unsaved drafts",
	"clear":  "Clear the screen",
	"whoami": "Show your identity and roles",
}

// Globals are resolved before editors, including help, so exit ? cannot exit.
func (s *Shell) globalCommand(fields []string) (handled, exit bool, err error) {
	command := fields[0]
	if command == "logout" || command == "quit" {
		command = "exit"
	}
	description, ok := globalDescriptions[command]
	if !ok {
		return false, false, nil
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
		if s.editorDirty() || len(s.stagedLogical) > 0 {
			s.writeTerminalData([]byte("Warning: Unsaved drafts were discarded before closing the session.\n"))
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
	s.writeTerminalData([]byte(fmt.Sprintf("Discarded unsaved edits for '%s'.\n", name)))
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
