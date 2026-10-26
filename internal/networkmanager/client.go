package networkmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"safeterm/internal/contract"
	"safeterm/internal/isolation"
)

type API interface {
	Call(context.Context, contract.Metadata, Request) (Reply, error)
}
type Client struct{ transport *isolation.UnixClient }

func NewClient(path string, uid uint32) (*Client, error) {
	client, err := isolation.NewUnixClient(path, []uint32{uid})
	if err != nil {
		return nil, err
	}
	return &Client{transport: client}, nil
}
func (c *Client) Call(ctx context.Context, actor contract.Metadata, r Request) (Reply, error) {
	if c == nil || c.transport == nil {
		return Reply{}, fmt.Errorf("network: client unavailable")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return Reply{}, err
	}
	request, err := contract.NewExecutionRequest("network_"+r.Action, map[string]string{"payload": string(data)}, actor)
	if err != nil {
		return Reply{}, err
	}
	response, err := c.transport.Handle(ctx, request)
	if err != nil {
		return Reply{}, err
	}
	if response.Result != contract.ResultSuccess {
		return Reply{}, fmt.Errorf("network: request failed")
	}
	var reply Reply
	if err := DecodeJSON([]byte(response.Stdout), &reply); err != nil {
		return reply, err
	}
	return reply, nil
}
