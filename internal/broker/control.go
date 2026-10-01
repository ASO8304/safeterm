// Package broker implements authenticated remote adapters and their shared
// transport-neutral control mappings.
package broker

import "safeterm/internal/contract"

type WebControlAdapter struct{}

func (WebControlAdapter) TerminateButton(sessionID, userID string) contract.ControlRequest {
	return terminate(sessionID, contract.SessionOwner{BrokerType: "web", UserID: userID})
}

type SSHControlAdapter struct{}

func (SSHControlAdapter) Interrupt(sessionID, userID string) contract.ControlRequest {
	return terminate(sessionID, contract.SessionOwner{BrokerType: "ssh", UserID: userID})
}

func terminate(sessionID string, owner contract.SessionOwner) contract.ControlRequest {
	return contract.ControlRequest{
		Version: contract.StreamProtocolVersion, Action: contract.ControlTerminate,
		SessionID: sessionID, Owner: owner,
	}
}
