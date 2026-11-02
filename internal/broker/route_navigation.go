package broker

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"

	"safeterm/internal/networkmanager"
)

func (e *networkEditSession) addRoute() {
	e.ensureRouteNumbers()
	for {
		name := "route-" + rand.Text()
		if _, used := e.routeNumbers[name]; used {
			continue
		}
		e.item = name
		e.ensureRouteNumbers()
		return
	}
}

// Selectors belong to this cached editor, not to persisted Netplan route names.
// Keep deleted selectors reserved so another route cannot silently take its ID.
func (e *networkEditSession) ensureRouteNumbers() {
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
	if e.kind == "route" && e.item != "" {
		names[e.item] = true
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
			if used[number] {
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

func (e *networkEditSession) protectedRoute(name string) bool {
	return networkmanager.IsDefaultRoute(e.base.Routes[name]) || networkmanager.IsDefaultRoute(e.view.Routes[name])
}

func (e *networkEditSession) deletableRoutes() []string {
	choices := []string{}
	for _, id := range e.itemChoices() {
		name, _ := e.resolveItem(id)
		if !e.protectedRoute(name) && e.view.Routes[name].ReadOnly == "" {
			choices = append(choices, id)
		}
	}
	return choices
}

func (e *networkEditSession) routeTitle(name string) string {
	e.ensureRouteNumbers()
	r, ok := e.view.Routes[name]
	if !ok {
		r, ok = e.base.Routes[name]
	}
	label := "Static route"
	if !ok {
		label = "new"
	}
	if r.Destination == "0.0.0.0/0" {
		label = "Default IPv4"
	}
	if r.Destination == "::/0" {
		label = "Default IPv6"
	}
	return e.routeNumbers[name] + " (" + label + ")"
}

func (e *networkEditSession) routeDescription(name string) string {
	r := e.view.Routes[name]
	via := "direct"
	if r.Gateway != "" {
		via = "via " + r.Gateway
	}
	description := fmt.Sprintf("%s %s on %s", r.Destination, via, r.Interface)
	if e.protectedRoute(name) {
		description += "; default route, deletion protected"
	}
	return description
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
	s.showRouteValidation(e, e.item)
	if e.protectedRoute(e.item) {
		s.networkStyledParagraph(styleWarning, "", "Deletion: Protected (default route)")
	}
}
