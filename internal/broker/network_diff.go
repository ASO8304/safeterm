package broker

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"safeterm/internal/networkmanager"
)

type editorFieldDiff struct {
	label         string
	before, after []string
}
type editorItemDiff struct {
	name   string
	fields []editorFieldDiff
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
func optionalNumber[T int | uint32](value *T) string {
	if value == nil {
		return "Automatic"
	}
	return fmt.Sprint(*value)
}

func interfaceDiff(before, after networkmanager.Interface) []editorFieldDiff {
	var fields []editorFieldDiff
	add := func(label string, old, next []string) {
		if !slices.Equal(old, next) {
			fields = append(fields, editorFieldDiff{label, old, next})
		}
	}
	add("DHCP4", []string{onOff(before.DHCP4)}, []string{onOff(after.DHCP4)})
	add("Addresses", before.Addresses, after.Addresses)
	add("DNS Servers", before.DNS, after.DNS)
	add("Search Domains", before.SearchDomains, after.SearchDomains)
	add("MTU", []string{optionalNumber(before.MTU)}, []string{optionalNumber(after.MTU)})
	return fields
}

func networkDiffs(e *networkEditSession) []editorItemDiff {
	var diffs []editorItemDiff
	if e.kind == "interface" {
		for name, item := range e.view.Interfaces {
			if fields := interfaceDiff(e.base.Interfaces[name], item); len(fields) > 0 {
				diffs = append(diffs, editorItemDiff{name, fields})
			}
		}
	} else {
		names := map[string]bool{}
		for name := range e.base.Routes {
			names[name] = true
		}
		for name := range e.view.Routes {
			names[name] = true
		}
		for name := range names {
			before, had := e.base.Routes[name]
			after, has := e.view.Routes[name]
			if had == has && reflect.DeepEqual(before, after) {
				continue
			}
			fields := []editorFieldDiff{}
			old := []string{before.Interface, before.Destination, before.Gateway, optionalNumber(before.Metric)}
			next := []string{after.Interface, after.Destination, after.Gateway, optionalNumber(after.Metric)}
			for i, label := range []string{"Interface", "Destination", "Gateway", "Metric"} {
				var a, b []string
				if had {
					a = []string{old[i]}
				}
				if has {
					b = []string{next[i]}
				}
				if !slices.Equal(a, b) {
					fields = append(fields, editorFieldDiff{label, a, b})
				}
			}
			diffs = append(diffs, editorItemDiff{name, fields})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].name < diffs[j].name })
	return diffs
}

func (s *Shell) showNetworkDiff(e *networkEditSession) {
	diffs := networkDiffs(e)
	if len(diffs) == 0 {
		s.writeTerminalData([]byte("No changes.\n"))
		return
	}
	for _, item := range diffs {
		kind := "Interface"
		if e.kind == "route" {
			kind = "Route"
		}
		s.writeTerminalData([]byte(fmt.Sprintf("%s: %s\n", kind, item.name)))
		for _, field := range item.fields {
			s.writeTerminalData([]byte("  " + field.label + ":\n"))
			s.diffValues(field.before, field.after)
		}
	}
}

// A bounded LCS keeps unchanged list entries as context, including reordering.
// Large/multiline application values fall back to full removal and addition.
func (s *Shell) diffValues(before, after []string) {
	write := func(prefix, value string) {
		for _, line := range strings.Split(value, "\n") {
			s.writeTerminalData([]byte(prefix + "   " + line + "\n"))
		}
	}
	if len(before)*len(after) > 4096 {
		for _, value := range before {
			write("-", value)
		}
		for _, value := range after {
			write("+", value)
		}
		return
	}
	lcs := make([][]int, len(before)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(after)+1)
	}
	for i := len(before) - 1; i >= 0; i-- {
		for j := len(after) - 1; j >= 0; j-- {
			if before[i] == after[j] {
				lcs[i][j] = 1 + lcs[i+1][j+1]
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(before) || j < len(after) {
		switch {
		case i < len(before) && j < len(after) && before[i] == after[j]:
			write(" ", before[i])
			i++
			j++
		case i < len(before) && (j == len(after) || lcs[i+1][j] >= lcs[i][j+1]):
			write("-", before[i])
			i++
		default:
			write("+", after[j])
			j++
		}
	}
}
