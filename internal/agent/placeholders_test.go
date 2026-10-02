package agent

import (
	"context"

	"safeterm/internal/contract"
)

// These permissive stages preserve the M2 pipeline test without exposing
// bypass implementations to production packages.
type passThroughValidator struct{}

func (passThroughValidator) Validate(*contract.ExecutionRequest) error { return nil }

type allowAllPolicy struct{}

func (allowAllPolicy) Authorize(*contract.ExecutionRequest) error { return nil }

type identityTranslator struct{}

func (identityTranslator) Translate(request *contract.ExecutionRequest) (string, []string, error) {
	return request.Command, nil, nil
}

type noOpExecutor struct{}

func (noOpExecutor) Execute(context.Context, string, []string) (string, string, int, error) {
	return "ok", "", 0, nil
}
