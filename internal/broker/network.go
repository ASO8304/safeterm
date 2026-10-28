package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"safeterm/internal/contract"
	"safeterm/internal/networkmanager"
)

type networkEditSession struct {
	kind, item string
	base, view networkmanager.View
	changes    []networkmanager.Change
}

func (s *Shell) SetNetworkClient(client networkmanager.API) { s.network = client }
func (s *Shell) networkCall(ctx context.Context, r networkmanager.Request) (networkmanager.Reply, error) {
	if s.network == nil {
		return networkmanager.Reply{}, fmt.Errorf("network management unavailable")
	}
	return s.network.Call(ctx, contract.Metadata{BrokerType: s.principal.BrokerType, UserID: s.principal.UserID, ConsoleSession: s.principal.ConsoleSession, ClaimedRoles: append([]string(nil), s.principal.Roles...), ReceivedAt: time.Now().UTC()}, r)
}

var networkActions = []string{"status", "diff", "apply", "confirm", "rollback", "discard"}

func (s *Shell) networkCommand(ctx context.Context, f []string) (bool, error) {
	if len(f) < 2 || f[1] != "network" || (f[0] != "execute" && f[0] != "config") {
		return false, nil
	}
	if s.editing != nil {
		return true, fmt.Errorf("leave the current configuration context first")
	}
	if f[0] == "execute" {
		if len(f) == 2 || (len(f) == 3 && f[2] == "?") {
			return true, s.networkActionHelp(nil)
		}
		if len(f) > 3 && f[len(f)-1] == "?" {
			return true, s.networkActionHelp(f[2 : len(f)-1])
		}
		if len(f) != 3 {
			return true, fmt.Errorf("usage: execute network ACTION")
		}
		r, err := s.networkCall(ctx, networkmanager.Request{Action: f[2]})
		if err == nil {
			s.writeTerminalData([]byte(r.Text + "\n"))
		}
		return true, err
	}
	if len(f) == 2 {
		s.configMenu = []string{"network"}
		return true, s.showConfigurationMenu(ctx)
	}
	if len(f) == 3 && f[2] == "?" {
		s.writeTerminalData([]byte("interface    Addresses, DHCP, DNS and MTU\nroute        Static routes\n"))
		return true, nil
	}
	if len(f) == 4 && f[3] == "?" && (f[2] == "interface" || f[2] == "route") {
		s.networkContextHelp(f[2], false)
		return true, nil
	}
	if len(f) != 3 || (f[2] != "interface" && f[2] != "route") {
		return true, fmt.Errorf("usage: config network interface|route")
	}
	r, err := s.networkCall(ctx, networkmanager.Request{Action: "read"})
	if err != nil {
		return true, err
	}
	if r.View == nil || r.View.Pending {
		return true, fmt.Errorf("network trial pending; use execute network confirm|rollback")
	}
	s.networkEdit = &networkEditSession{kind: f[2], base: *r.View, view: *r.View}
	s.configMenu = nil
	s.writeTerminalData([]byte("back discards edits to the item you leave; next keeps them staged. Only end saves a candidate. execute network apply starts a timed trial.\n"))
	return true, s.networkSelectionMenu()
}

func (s *Shell) networkSelectionMenu() error {
	s.networkContextHelp(s.networkEdit.kind, false)
	s.writeTerminalData([]byte("Available items (use edit NAME):\n"))
	return s.networkEditorHelp([]string{"edit"})
}
func (s *Shell) networkEditor(ctx context.Context, line string) error {
	e := s.networkEdit
	f := strings.Fields(line)
	if len(f) == 0 {
		return nil
	}
	if f[len(f)-1] == "?" {
		return s.networkEditorHelp(f[:len(f)-1])
	}

	switch f[0] {
	case "back":
		if len(f) != 1 {
			return fmt.Errorf("usage: back")
		}
		if e.item == "" {
			s.networkEdit = nil
			s.writeTerminalData([]byte("Unsaved edits in this network context discarded. Nothing saved.\n"))
			return s.openConfigurationPath(ctx, []string{"network"}, false)
		}
		remaining := make([]networkmanager.Change, 0, len(e.changes))
		for _, change := range e.changes {
			if change.Kind != e.kind || change.Name != e.item {
				remaining = append(remaining, change)
			}
		}
		view, err := networkmanager.Modify(e.base, remaining)
		if err != nil {
			return err
		}
		s.writeTerminalData([]byte(fmt.Sprintf("Unsaved edits to %s discarded. Nothing saved.\n", e.item)))
		e.changes, e.view, e.item = remaining, view, ""
		return s.networkSelectionMenu()
	case "abort", "cancel":
		if len(f) != 1 {
			break
		}
		s.networkEdit = nil
		s.writeTerminalData([]byte("Unsaved editor changes discarded; any previously saved candidate is kept.\n"))
		return s.questionHelp(ctx, nil)
	case "edit":
		if len(f) != 2 {
			break
		}
		if e.kind == "interface" {
			if _, ok := e.view.Interfaces[f[1]]; !ok {
				return fmt.Errorf("network: unknown interface")
			}
		}
		e.item = f[1]
		s.networkContextHelp(e.kind, true)
		return nil
	case "next":
		if len(f) != 1 {
			break
		}
		e.item = ""
		s.writeTerminalData([]byte("Edits kept in this editor; nothing saved.\n"))
		return s.networkSelectionMenu()
	case "show":
		if len(f) != 1 {
			break
		}
		if e.kind == "route" {
			s.showNetworkRoutes(e)
			return nil
		}
		s.showNetworkInterfaces(e)
		return nil
	case "diff":
		if len(f) != 1 {
			break
		}
		data, _ := json.MarshalIndent(e.changes, "", "  ")
		s.writeTerminalData(append(data, '\n'))
		return nil
	case "end":
		if len(f) != 1 {
			break
		}
		r, err := s.networkCall(ctx, networkmanager.Request{Action: "save", Version: e.base.Version, Changes: e.changes})
		if err != nil {
			return err
		}
		s.networkEdit = nil
		s.writeTerminalData([]byte(r.Text + "\nCandidate saved; use execute network diff, then execute network apply.\n"))
		return nil
	case "set", "append", "unselect", "unset":
		if e.item == "" {
			return fmt.Errorf("use edit NAME first")
		}
		if len(f) < 2 {
			break
		}
		c := networkmanager.Change{Kind: e.kind, Name: e.item, Field: f[1], Action: f[0], Values: f[2:]}
		v, err := networkmanager.Modify(e.view, []networkmanager.Change{c})
		if err != nil {
			return err
		}
		e.view = v
		e.changes = append(e.changes, c)
		return nil
	case "delete":
		if len(f) != 2 || e.kind != "route" {
			break
		}
		c := networkmanager.Change{Kind: "route", Name: f[1], Action: "delete"}
		v, err := networkmanager.Modify(e.view, []networkmanager.Change{c})
		if err != nil {
			return err
		}
		e.view = v
		e.changes = append(e.changes, c)
		return nil
	}
	return fmt.Errorf("network: invalid command; use ?")
}
func (s *Shell) networkCompletion(before, prefix string) ([]string, bool) {
	f := strings.Fields(before)
	if !strings.HasSuffix(before, " ") && len(f) > 0 {
		f = f[:len(f)-1]
	}
	if e := s.networkEdit; e != nil {
		values := []string{"?", "back", "edit", "show", "diff", "next", "end", "abort"}
		if e.kind == "route" {
			values = append(values, "delete")
		}
		if e.item != "" {
			values = append(values, "set")
			if e.kind == "interface" {
				values = append(values, "append", "unselect", "unset")
			}
		}
		if len(f) > 0 {
			values = nil
			switch f[0] {
			case "edit", "delete":
				if len(f) == 1 {
					if e.kind == "interface" {
						for name := range e.view.Interfaces {
							values = append(values, name)
						}
					} else {
						for name := range e.view.Routes {
							values = append(values, name)
						}
					}
				}
			case "set", "unset", "append", "unselect":
				if len(f) == 1 {
					if e.kind == "interface" {
						values = []string{"dhcp4", "addresses", "dns", "search-domains", "mtu"}
						if f[0] == "append" || f[0] == "unselect" {
							values = []string{"addresses", "dns", "search-domains"}
						}
					} else {
						values = []string{"interface", "destination", "gateway", "metric"}
					}
				} else if len(f) == 2 && f[1] == "dhcp4" {
					values = []string{"enable", "disable"}
				} else if len(f) == 2 && f[1] == "interface" {
					for name := range e.view.Interfaces {
						values = append(values, name)
					}
				}
			}
		}
		sort.Strings(values)
		return matching(values, prefix), true
	}
	if s.network == nil {
		return nil, false
	}
	if len(f) == 2 && (f[0] == "config" || f[0] == "execute") && f[1] == "network" {
		if f[0] == "config" {
			return matching([]string{"interface", "route"}, prefix), true
		}
		return matching(networkActions, prefix), true
	}
	if len(f) == 1 && (f[0] == "config" || f[0] == "execute") {
		// Merge with the existing configurable command tree, preserving Linux Tab behavior.
		var values []string
		if f[0] == "config" {
			values = s.configCompletion(context.Background(), before, prefix)
		} else {
			values, _ = s.treeCompletion(context.Background(), before, prefix)
		}
		values = append(values, matching([]string{"network"}, prefix)...)
		sort.Strings(values)
		return values, true
	}
	return nil, false
}
