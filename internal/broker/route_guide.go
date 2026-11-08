package broker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"safeterm/internal/networkmanager"
)

func (s *Shell) networkEditorInteractive(ctx context.Context, line string, input <-chan byte) error {
	f := strings.Fields(line)
	if s.networkEdit.kind == "route" && input != nil && (len(f) == 1 && f[0] == "add" || len(f) == 2 && f[0] == "edit" && f[1] != "?") {
		return s.guideRoute(ctx, f, input)
	}
	return s.networkEditor(ctx, line)
}

func (s *Shell) guideRoute(ctx context.Context, fields []string, input <-chan byte) error {
	current := s.networkEdit
	work := *current
	work.routeNumbers = map[string]string{}
	for key, value := range current.routeNumbers {
		work.routeNumbers[key] = value
	}
	if fields[0] == "add" {
		work.addRoute()
	} else {
		name, ok := work.resolveItem(fields[1])
		if !ok {
			return fmt.Errorf("route ID not found; use show to list routes")
		}
		work.item = name
		if reason := work.view.Routes[name].ReadOnly; reason != "" {
			return fmt.Errorf("route is read-only: %s", reason)
		}
	}
	choices := []string{}
	for name, item := range work.view.Interfaces {
		if item.ReadOnly == "" {
			choices = append(choices, name)
		}
	}
	sort.Strings(choices)
	if len(choices) == 0 {
		return fmt.Errorf("no editable present interfaces; check network/policy.json")
	}
	s.writeStyled(styleHeading, "Route "+work.routeTitle(work.item)+"\n")
	s.writeTerminalData([]byte("Enter keeps the displayed value. Type cancel or press Ctrl+C to leave this guide unchanged.\n"))
	old := work.view.Routes[work.item]
	metric := optionalNumber(old.Metric)
	if old.Metric == nil {
		metric = "auto"
	}
	iface := old.Interface
	if iface == "" && len(choices) == 1 {
		iface = choices[0]
	}
	defaults := map[string]string{"destination": old.Destination, "gateway": old.Gateway, "interface": iface, "metric": metric}
	if defaults["gateway"] == "" {
		defaults["gateway"] = "direct"
	}
	labels := map[string]string{"destination": "Destination CIDR (or default4/default6)", "gateway": "Gateway IP (or direct)", "interface": "Interface (" + strings.Join(choices, ", ") + ")", "metric": "Metric (or auto)"}
	for _, field := range []string{"destination", "gateway", "interface", "metric"} {
		for {
			prompt := labels[field]
			if defaults[field] != "" {
				prompt += " [" + defaults[field] + "]"
			}
			prompt += ": "
			value, err := s.readLine(ctx, input, s.styled(styleCommand, prompt), false, false)
			if err != nil {
				s.writeStyled(styleWarning, "Route guide cancelled; previous edits kept.\n")
				return nil
			}
			value = strings.TrimSpace(value)
			if value == "cancel" {
				s.writeStyled(styleWarning, "Route guide cancelled; previous edits kept.\n")
				return nil
			}
			if value == "?" {
				s.networkHelpRow(field, networkRouteFields[field])
				continue
			}
			if value == "" {
				value = defaults[field]
			}
			if field == "destination" {
				if value == "default" || value == "default4" {
					value = "0.0.0.0/0"
				}
				if value == "default6" {
					value = "::/0"
				}
			}
			change := networkmanager.Change{Kind: "route", Name: work.item, Action: "set", Field: field, Values: []string{value}}
			view, err := networkmanager.Modify(work.view, []networkmanager.Change{change})
			if err != nil {
				s.writeStyled(styleError, fmt.Sprint(err)+"\n")
				continue
			}
			if field == "gateway" && view.Routes[work.item].Gateway != "" {
				// Validate family against the destination before advancing.
				r := view.Routes[work.item]
				r.Interface = choices[0]
				if err := networkmanager.ValidateRoute(view, r); err != nil {
					s.writeStyled(styleError, err.Error()+"\n")
					continue
				}
			}
			work.view = view
			work.changes = append(append([]networkmanager.Change(nil), work.changes...), change)
			break
		}
	}
	if err := networkmanager.ValidateRoute(work.view, work.view.Routes[work.item]); err != nil {
		return err
	}
	*s.networkEdit = work
	s.showNetworkRoute(s.networkEdit)
	s.writeTerminalData([]byte("Draft ready. Use diff to review, save to commit the candidate, or exit to discard this item's edits.\n"))
	return nil
}
