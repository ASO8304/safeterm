package broker

import (
	"context"
	"strings"
)

// Confirm only drafts that this navigation step would actually discard.
func (s *Shell) exitWouldDiscard() bool {
	if e := s.networkEdit; e != nil {
		for _, d := range networkDiffs(e) {
			if e.item == "" || d.name == e.item {
				return true
			}
		}
		return false
	}
	if e := s.editing; e != nil {
		for _, c := range e.changes {
			if e.logical || !e.inSection || c.Section == e.section {
				return true
			}
		}
		return false
	}
	for _, e := range s.stagedLogical {
		if s.configMenu == nil || wordsStart(e.path, s.configMenu) {
			return true
		}
	}
	return false
}

func (s *Shell) confirmDiscard(ctx context.Context, input <-chan byte) bool {
	if input == nil {
		s.writeStyled(styleWarning, "Unsaved edits kept. Use 'next' to keep them staged or 'save' to commit.\n")
		return false
	}
	answer, err := s.readLine(ctx, input, s.styled(styleWarning, "Discard unsaved edits and exit this level? [y/N]: "), false, false)
	if err == nil && (strings.EqualFold(strings.TrimSpace(answer), "yes") || strings.EqualFold(strings.TrimSpace(answer), "y")) {
		return true
	}
	s.writeStyled(styleWarning, "Unsaved edits kept; exit cancelled.\n")
	return false
}
