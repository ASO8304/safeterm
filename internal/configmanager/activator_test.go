package configmanager

import (
	"context"
	"testing"

	"safeterm/internal/contract"
)

type activationClient struct {
	request *contract.ExecutionRequest
	result  contract.ExecutionResult
}

func (c *activationClient) Handle(_ context.Context, request *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	c.request = request
	return &contract.ExecutionResponse{RequestID: request.RequestID, Result: c.result}, nil
}
func TestGatewayActivatorBuildsTypedArgumentFreeOperation(t *testing.T) {
	client := &activationClient{result: contract.ResultSuccess}
	activator, _ := NewGatewayActivator(client)
	if err := activator.Run(context.Background(), actor("alice", "operator"), "reload-cats"); err != nil {
		t.Fatal(err)
	}
	if client.request.Command != "reload-cats" || len(client.request.Arguments) != 0 || client.request.Metadata.BrokerType != "config-manager" || client.request.Metadata.UserID != "alice" {
		t.Fatalf("request=%+v", client.request)
	}
}
