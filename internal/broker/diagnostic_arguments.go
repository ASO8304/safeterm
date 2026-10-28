package broker

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"safeterm/internal/config"
)

func positionalDiagnostic(op config.OperationDefinition) bool {
	return op.Name == "ping" || op.Name == "telnet" || op.Name == "tcp-test"
}
func parseDiagnosticArguments(op config.OperationDefinition, fields []string) (map[string]string, error) {
	result := map[string]string{}
	usage := "usage: execute tcp-test IP [PORT]"
	if op.Name == "ping" {
		usage = "usage: execute ping IP [-c COUNT]"
	}
	for i := 0; i < len(fields); i++ {
		token := fields[i]
		if op.Name == "ping" && token == "-c" {
			if result["count"] != "" {
				return nil, fmt.Errorf("ping: -c may be specified only once")
			}
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("ping: -c requires a count from 1 to 1000")
			}
			i++
			value := fields[i]
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 1000 {
				return nil, fmt.Errorf("ping: count must be between 1 and 1000")
			}
			result["count"] = strconv.Itoa(n)
			continue
		}
		if strings.HasPrefix(token, "-") || strings.Contains(token, "=") {
			return nil, fmt.Errorf("%s", usage)
		}
		if result["host"] == "" {
			ip, err := netip.ParseAddr(token)
			if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() || (op.Name == "ping" && !ip.Is4()) {
				return nil, fmt.Errorf("%s; supply a literal %sIP address", usage, map[bool]string{true: "IPv4 ", false: ""}[op.Name == "ping"])
			}
			result["host"] = ip.String()
			continue
		}
		if op.Name != "ping" && result["port"] == "" {
			n, err := strconv.Atoi(token)
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("tcp-test: port must be between 1 and 65535")
			}
			result["port"] = strconv.Itoa(n)
			continue
		}
		return nil, fmt.Errorf("%s", usage)
	}
	if result["host"] == "" {
		return nil, fmt.Errorf("%s; IP is required", usage)
	}
	return result, nil
}
func diagnosticCompletion(op config.OperationDefinition, completed []string, prefix string) []string {
	if op.Name != "ping" {
		return nil
	}
	flagSeen := false
	expectCount := false
	for _, token := range completed {
		if expectCount {
			expectCount = false
			continue
		}
		if token == "-c" {
			flagSeen = true
			expectCount = true
		}
	}
	if expectCount {
		return matching([]string{"1", "5", "10", "100", "1000"}, prefix)
	}
	if !flagSeen {
		return matching([]string{"-c"}, prefix)
	}
	return nil
}
func (s *Shell) diagnosticHelp(op config.OperationDefinition, args []string) {
	if op.Name == "ping" {
		s.networkHelpRow("IP", "Required IPv4 destination; example: execute ping 192.0.2.1")
		s.networkHelpRow("-c COUNT", "Optional number of requests, 1..1000; default 1000; before or after IP")
		if len(args) > 0 && args[len(args)-1] == "-c" {
			s.networkHelpRow("<1..1000>", "Number of requests; larger values are rejected")
		}
	} else {
		defaultPort := "23"
		for _, p := range op.Parameters {
			if p.Name == "port" && p.Default != "" {
				defaultPort = p.Default
			}
		}
		s.networkHelpRow("IP", "Required literal IPv4 or IPv6 address; no login or application data")
		s.networkHelpRow("PORT", "Optional TCP port, 1..65535; default "+defaultPort)
		if op.TCPProbe != nil {
			s.networkHelpRow("timeout", fmt.Sprintf("Connection attempt ends after at most %d seconds", op.TCPProbe.TimeoutSeconds))
		}
	}
	s.networkHelpRow("Ctrl+C", "Cancel this operation and return to SafeTerm")
	s.networkHelpRow("Ctrl+]", "Close the entire SafeTerm session")
}
