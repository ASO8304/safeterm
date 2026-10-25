package audit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var serviceUnitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*\.service$`)
var serviceActionPattern = regexp.MustCompile(`^(status|start|stop|restart|reload|enable|disable)$`)
var serviceOperationPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// ServiceStep identifies a child operation inside a signed service sequence.
type ServiceStep struct {
	ParentOperation string
	Index           int
	Action          string
	Unit            string
}

// ServiceStepOperation keeps the V1 audit envelope, existing accepted/completed
// outcomes and canonical serialization intact. Older readers can validate the
// chain after a package rollback. Colons cannot occur in validated input tokens.
func ServiceStepOperation(parent string, index int, action, unit string) (string, error) {
	if !serviceOperationPattern.MatchString(parent) || index < 1 || index > 64 || !serviceUnitPattern.MatchString(unit) || strings.Contains(unit, "..") || !serviceActionPattern.MatchString(action) {
		return "", fmt.Errorf("audit: invalid service step identity")
	}
	return fmt.Sprintf("service-step:%s:%d:%s:%s", parent, index, action, unit), nil
}

func ParseServiceStep(operation string) (ServiceStep, bool) {
	parts := strings.SplitN(operation, ":", 6)
	if len(parts) != 5 || parts[0] != "service-step" {
		return ServiceStep{}, false
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil {
		return ServiceStep{}, false
	}
	canonical, err := ServiceStepOperation(parts[1], index, parts[3], parts[4])
	if err != nil || canonical != operation {
		return ServiceStep{}, false
	}
	return ServiceStep{ParentOperation: parts[1], Index: index, Action: parts[3], Unit: parts[4]}, true
}
