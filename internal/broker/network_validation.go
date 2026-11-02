package broker

import (
	"errors"
	"fmt"

	"safeterm/internal/networkmanager"
)

func (e *networkEditSession) validationError(err error) error {
	var issue *networkmanager.RouteValidationError
	if !errors.As(err, &issue) {
		return err
	}
	label := "Route " + e.routeTitle(issue.Route.Name)
	if issue.Route.Interface != "" {
		label += " on interface " + issue.Route.Interface
	}
	return fmt.Errorf("%s: %s", label, issue.Problem)
}

func (e *networkEditSession) saveValidationError(err error) error {
	var issue *networkmanager.RouteValidationError
	if errors.As(err, &issue) {
		action := "correct the invalid route in config network route"
		if e.kind == "route" {
			e.ensureRouteNumbers()
			action = "use edit " + e.routeNumbers[issue.Route.Name] + " to correct this route"
		}
		return fmt.Errorf("cannot save network candidate: %v. All interface and route settings are validated together; %s", e.validationError(err), action)
	}
	return fmt.Errorf("cannot save network candidate: %w", err)
}

func (s *Shell) showRouteValidation(e *networkEditSession, name string) {
	route, exists := e.view.Routes[name]
	if !exists {
		return
	}
	if err := networkmanager.ValidateRoute(e.view, route); err != nil {
		s.networkStyledParagraph(styleWarning, "", "Invalid configuration: "+e.validationError(err).Error())
	}
}
