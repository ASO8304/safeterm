package config

import (
	"fmt"
	"regexp"
	"strings"
)

var serviceAliasPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

// ServiceStep is a fixed, signed unit/action pair, never a caller argument.
type ServiceStep struct {
	Unit   string
	Action string
}

func (s SystemdOperationDefinition) OrderedUnits() []string {
	if s.Unit != "" {
		return []string{s.Unit}
	}
	return append([]string(nil), s.Units...)
}

func (s SystemdOperationDefinition) Timeout() int {
	if s.TimeoutSeconds == 0 {
		return 120
	}
	return s.TimeoutSeconds
}

// Steps must only be used after catalog validation. Units declare startup order.
func (s SystemdOperationDefinition) Steps() []ServiceStep {
	units := s.OrderedUnits()
	steps := []ServiceStep{}
	appendSteps := func(action string, reverse bool) {
		for i := range units {
			index := i
			if reverse {
				index = len(units) - 1 - i
			}
			steps = append(steps, ServiceStep{Unit: units[index], Action: action})
		}
	}
	if len(s.Units) > 0 && s.Action == "restart" {
		appendSteps("stop", true)
		appendSteps("start", false)
	} else {
		appendSteps(s.Action, s.Action == "stop" || s.Action == "disable")
	}
	return steps
}

func ValidateSystemdDefinition(s *SystemdOperationDefinition) error {
	if s == nil || !validSystemdAction(s.Action) {
		return fmt.Errorf("invalid service action")
	}
	if (s.Unit == "") == (len(s.Units) == 0) {
		return fmt.Errorf("specify exactly one of unit or nonempty units")
	}
	if len(s.Units) > 32 {
		return fmt.Errorf("service group exceeds 32 units")
	}
	seen := map[string]bool{}
	for _, unit := range s.OrderedUnits() {
		if !unitPattern.MatchString(unit) || strings.Contains(unit, "..") || seen[unit] {
			return fmt.Errorf("invalid or duplicate service unit %q", unit)
		}
		seen[unit] = true
	}
	if s.Alias != "" && !serviceAliasPattern.MatchString(s.Alias) {
		return fmt.Errorf("invalid service alias")
	}
	if s.TimeoutSeconds < 0 || s.TimeoutSeconds > 240 {
		return fmt.Errorf("service timeout_seconds must be 1..240, or omitted")
	}
	return nil
}
