package broker

import (
	"bytes"
	"fmt"
	"sort"
	"text/tabwriter"

	"safeterm/internal/networkmanager"
)

// This is the configured candidate, not the kernel's DHCP/learned routing table.
// Display uses the cached editor view and never contacts or changes the network.
func (s *Shell) showNetworkRoutes(e *networkEditSession) {
	groups := map[string][]networkmanager.Route{}
	if e.item == "" {
		for name := range e.view.Interfaces {
			groups[name] = nil
		}
	}
	if e.item != "" {
		if _, exists := e.view.Routes[e.item]; !exists {
			s.writeTerminalData([]byte(fmt.Sprintf("Route %s has no fields yet. Use set interface NAME and set destination NETWORK/CIDR; gateway and metric are optional.\n", e.item)))
			return
		}
	}
	for name, route := range e.view.Routes {
		if e.item != "" && name != e.item {
			continue
		}
		route.Name = name
		groups[route.Interface] = append(groups[route.Interface], route)
	}
	s.writeTerminalData([]byte("Configured routes (candidate)\n"))
	if len(groups) == 0 {
		s.writeTerminalData([]byte("No configured interfaces or routes.\n"))
		return
	}
	names := []string{}
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		heading := "Interface " + name
		if name == "" {
			heading = "Unassigned routes (use set interface NAME)"
		}
		if item, exists := e.view.Interfaces[name]; exists && item.ReadOnly != "" {
			heading += " [read-only: " + item.ReadOnly + "]"
		}
		s.writeTerminalData([]byte(heading + "\n"))
		routes := groups[name]
		sort.Slice(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
		if len(routes) == 0 {
			s.writeTerminalData([]byte("  No entries in the Netplan routes list.\n"))
			continue
		}
		var table bytes.Buffer
		writer := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "  NAME\tDESTINATION\tGATEWAY\tMETRIC")
		for _, route := range routes {
			destination := route.Destination
			if destination == "" {
				destination = "<not set>"
			}
			gateway := route.Gateway
			if gateway == "" {
				gateway = "-"
			}
			metric := "auto"
			if route.Metric != nil {
				metric = fmt.Sprint(*route.Metric)
			}
			label := route.Name
			if route.ReadOnly != "" {
				label += " [read-only]"
			}
			fmt.Fprintf(writer, "  %s\t%s\t%s\t%s\n", label, destination, gateway, metric)
		}
		_ = writer.Flush()
		s.writeTerminalData(table.Bytes())
		total += len(routes)
	}
	if total == 0 {
		s.writeTerminalData([]byte("Use edit NAME to create a route, then set interface NAME and set destination NETWORK/CIDR.\n"))
	}
}
