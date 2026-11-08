package broker

import (
	"strings"

	"safeterm/internal/contract"
)

func (s *Shell) writeOperationResponse(response *contract.ExecutionResponse, canceled bool, probeOutput string) {
	if s.presentation.ShowResult || response == nil {
		s.writeResponse(response)
		return
	}
	copy := *response
	s.writeTerminalData([]byte(copy.Stdout))
	s.writeTerminalData([]byte(copy.Stderr))
	copy.Stdout, copy.Stderr = "", ""
	response = &copy
	if canceled && response.Result == contract.ResultExecutionError &&
		(response.ExitCode == 130 || response.ExitCode == -1 && (strings.Contains(response.Message, "semantic terminate requested") || strings.Contains(response.Message, "was canceled") || strings.Contains(response.Message, "Connection test canceled."))) &&
		!strings.Contains(strings.ToLower(response.Message), "audit") {
		s.writeStyled(styleWarning, "Operation stopped.\n")
		return
	}
	// A typed port probe already explains closed/unreachable/inconclusive states.
	// Match the final reason too, so an unrelated runner/audit failure is visible.
	if response.Result == contract.ResultExecutionError && response.FailureStage == "execution" {
		for _, line := range strings.Split(probeOutput, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "Trying ") && strings.HasSuffix(response.Message, line) {
				return
			}
		}
	}
	s.writeResponse(response)
}
