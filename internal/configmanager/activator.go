package configmanager

import (
	"context"
	"fmt"
	"time"

	"safeterm/internal/contract"
)

type ExecutionClient interface {
	Handle(context.Context, *contract.ExecutionRequest) (*contract.ExecutionResponse, error)
}
type GatewayActivator struct{ client ExecutionClient }

func NewGatewayActivator(client ExecutionClient) (*GatewayActivator, error) {
	if client == nil {
		return nil, fmt.Errorf("configmanager: activation client is required")
	}
	return &GatewayActivator{client: client}, nil
}
func (a *GatewayActivator) Run(ctx context.Context, actor Actor, operation string) error {
	request, err := contract.NewExecutionRequest(operation, map[string]string{}, contract.Metadata{BrokerType: "config-manager", UserID: actor.UserID, ClaimedRoles: append([]string(nil), actor.Roles...), ReceivedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	response, err := a.client.Handle(ctx, request)
	if err != nil {
		return err
	}
	if response == nil || response.Result != contract.ResultSuccess {
		return fmt.Errorf("configmanager: typed activation operation %q failed", operation)
	}
	return nil
}
