package networkmanager

import (
	"crypto/sha256"
	"fmt"
	"os"
)

// Drafts are isolated by authenticated transport and user, never by client input
// paths. Live trials remain globally serialized and owned by their initiator.
func candidateFile(identity string) string {
	return fmt.Sprintf("candidate-%x.json", sha256.Sum256([]byte(identity)))
}

func (m *Manager) loadCandidate(identity string, out *candidate) error {
	err := m.load(candidateFile(identity), out)
	if !os.IsNotExist(err) {
		return err
	}
	// Preserve the old global candidate for its owner during upgrades. Another
	// identity's old draft must not block this identity's independent editor.
	var legacy candidate
	if err := m.load("candidate.json", &legacy); err != nil {
		return err
	}
	if legacy.Owner != identity {
		return os.ErrNotExist
	}
	if err := m.save(candidateFile(identity), legacy); err != nil {
		return err
	}
	if err := m.remove("candidate.json"); err != nil {
		return err
	}
	*out = legacy
	return nil
}

func (m *Manager) removeCandidate(identity string) error {
	if err := m.remove(candidateFile(identity)); err != nil {
		return err
	}
	var legacy candidate
	if err := m.load("candidate.json", &legacy); err == nil && legacy.Owner == identity {
		return m.remove("candidate.json")
	}
	return nil
}
