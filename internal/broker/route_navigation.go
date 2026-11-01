package broker

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func numericRouteName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Selectors belong to this cached editor, not to persisted Netplan route names.
// Keep deleted selectors reserved so another route cannot silently take its ID.
func (e *networkEditSession) ensureRouteNumbers() {
	if e.kind != "route" {
		return
	}
	if e.routeNumbers == nil {
		e.routeNumbers = map[string]string{}
	}
	names := map[string]bool{}
	for name := range e.base.Routes {
		names[name] = true
	}
	for name := range e.view.Routes {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	used := map[string]bool{}
	for _, number := range e.routeNumbers {
		used[number] = true
	}
	for _, name := range ordered {
		if e.routeNumbers[name] != "" {
			continue
		}
		for n := 1; ; n++ {
			number := strconv.Itoa(n)
			if used[number] || (names[number] && number != name) {
				continue
			}
			e.routeNumbers[name], used[number] = number, true
			break
		}
	}
}

func (e *networkEditSession) resolveItem(value string) (string, bool) {
	if e.kind == "interface" {
		_, ok := e.view.Interfaces[value]
		return value, ok
	}
	e.ensureRouteNumbers()
	if _, ok := e.view.Routes[value]; ok {
		return value, true
	}
	for name, number := range e.routeNumbers {
		if number == value {
			_, ok := e.view.Routes[name]
			return name, ok
		}
	}
	return "", false
}

func (e *networkEditSession) itemChoices() []string {
	choices := []string{}
	if e.kind == "interface" {
		for name := range e.view.Interfaces {
			choices = append(choices, name)
		}
		sort.Strings(choices)
	} else {
		e.ensureRouteNumbers()
		for name := range e.view.Routes {
			choices = append(choices, e.routeNumbers[name])
		}
		sort.Slice(choices, func(i, j int) bool { a, _ := strconv.Atoi(choices[i]); b, _ := strconv.Atoi(choices[j]); return a < b })
	}
	return choices
}

func generatedRouteName(name string) bool {
	if !strings.HasPrefix(name, "route-") || len(name) != 18 {
		return false
	}
	_, err := hex.DecodeString(name[6:])
	return err == nil
}

func (e *networkEditSession) routeTitle(name string) string {
	e.ensureRouteNumbers()
	label := name
	if generatedRouteName(name) {
		r, ok := e.view.Routes[name]
		if !ok {
			r = e.base.Routes[name]
		}
		label = "Static route"
		if r.Destination == "0.0.0.0/0" {
			label = "Default IPv4"
		}
		if r.Destination == "::/0" {
			label = "Default IPv6"
		}
	}
	if number := e.routeNumbers[name]; number != "" {
		return number + " (" + label + ")"
	}
	return label + " (new)"
}

func (e *networkEditSession) routeDescription(name string) string {
	r := e.view.Routes[name]
	via := "direct"
	if r.Gateway != "" {
		via = "via " + r.Gateway
	}
	label := ""
	if !generatedRouteName(name) {
		label = name + ": "
	}
	return fmt.Sprintf("%s%s %s on %s", label, r.Destination, via, r.Interface)
}

func (s *Shell) showNetworkRoute(e *networkEditSession) {
	r := e.view.Routes[e.item]
	s.writeStyled(styleHeading, "Route: "+e.routeTitle(e.item)+"\n")
	for _, d := range networkDiffs(e) {
		if d.name == e.item {
			s.writeStyled(styleWarning, "* Unsaved edits staged\n")
			break
		}
	}
	s.writeTerminalData([]byte("\n"))
	value := func(v string) string {
		if v == "" {
			return "Not set"
		}
		return v
	}
	for _, field := range [][2]string{{"Destination", value(r.Destination)}, {"Gateway", value(r.Gateway)}, {"Interface", value(r.Interface)}, {"Metric", optionalNumber(r.Metric)}} {
		if field[0] == "Gateway" && r.Gateway == "" {
			field[1] = "Direct (no gateway)"
		}
		s.writeStyled(styleCommand, fmt.Sprintf("%-16s", field[0]+":"))
		s.writeTerminalData([]byte(field[1] + "\n"))
	}
	if r.ReadOnly != "" {
		s.networkStyledParagraph(styleWarning, "", "Read-only: "+r.ReadOnly)
	}
}
