package broker

import (
	"fmt"
	"os"
	"strings"
)

// Use the terminal's own ANSI palette so light and dark profiles remain usable.
const (
	styleHeading = "1;36"
	styleCommand = "36"
	styleSuccess = "32"
	stylePath    = "1;34"
	styleWarning = "33"
	styleError   = "1;31"
)

// SetTerminal configures presentation before Run. Callers without an interactive
// terminal retain plain output. NO_COLOR is a broker-process environment setting.
func (s *Shell) SetTerminal(terminal string) {
	s.color = terminal != "" && terminal != "dumb" && os.Getenv("NO_COLOR") == ""
}

// Only these trusted style codes can introduce ANSI escapes. Text still goes
// through the terminal sanitizer, including resource names and API error text.
func (s *Shell) styled(style, value string) string {
	var safe strings.Builder
	plain := Shell{output: &safe}
	plain.writeTerminalData([]byte(value))
	if !s.color || value == "" {
		return safe.String()
	}
	switch style {
	case styleHeading, styleCommand, styleSuccess, stylePath, styleWarning, styleError:
	default:
		return safe.String()
	}
	return "\x1b[" + style + "m" + safe.String() + "\x1b[0m"
}

func (s *Shell) writeStyled(style, value string) {
	fmt.Fprint(s.output, s.styled(style, value))
}

func (s *Shell) formatPrompt(path, dirty string) string {
	return s.styled(styleHeading, "safeterm") + "::" +
		s.styled(styleSuccess, s.principal.UserID) + s.styled(stylePath, path) +
		s.styled(styleWarning, dirty) + s.styled(styleCommand, ">") + " "
}

func (s *Shell) helpRow(name, description string) {
	s.writeStyled(styleCommand, fmt.Sprintf("%-24s", name))
	s.writeTerminalData([]byte(" " + description + "\n"))
}

// Style only after tabwriter has measured the plain table, preserving columns.
func (s *Shell) writeTable(data []byte) {
	header, rest, _ := strings.Cut(string(data), "\n")
	s.writeStyled(styleHeading, header)
	s.writeTerminalData([]byte("\n"))
	columns := strings.Fields(header)
	starts := make([]int, len(columns))
	offset := 0
	for i, name := range columns {
		starts[i] = offset + strings.Index(header[offset:], name)
		offset = starts[i] + len(name)
	}
	for _, line := range strings.SplitAfter(rest, "\n") {
		offset = 0
		for i, name := range columns {
			start := starts[i]
			if start >= len(line) {
				break
			}
			end := len(line)
			if i+1 < len(starts) && starts[i+1] < end {
				end = starts[i+1]
			}
			s.writeTerminalData([]byte(line[offset:start]))
			cell := line[start:end]
			style := ""
			switch name {
			case "NAME":
				style = styleCommand
			case "STATE":
				if strings.TrimSpace(cell) == "up" {
					style = styleSuccess
				} else {
					style = styleWarning
				}
			case "EDITING":
				style = styleSuccess
				if strings.HasPrefix(cell, "Read-only") {
					style = styleWarning
				}
			}
			if name == "NAME" && strings.Contains(cell, " *") {
				before, after, _ := strings.Cut(cell, " *")
				s.writeStyled(style, before)
				s.writeStyled(styleWarning, " *")
				s.writeTerminalData([]byte(after))
			} else {
				s.writeStyled(style, cell)
			}
			offset = end
		}
		s.writeTerminalData([]byte(line[offset:]))
	}
}
