# ADR-002: ExecutionRequest / ExecutionResponse Contract Design

## Status
Accepted

## Context
Broker and Agent need a single, stable data contract for communication (REQ-001).
Several design decisions were required:

1. How should a unique identifier (`RequestID`) be generated?
2. Should the project use an external library such as UUID or only the Go standard library?
3. Will Broker and Agent always run in one process, or might they later become separate services or processes?

## Decision
- `RequestID` is generated with `crypto/rand` using 16 random bytes encoded as
  hexadecimal, rather than an external UUID library.
- `ExecutionRequest` and `ExecutionResponse` both have JSON tags and must
  survive Marshal/Unmarshal without data loss, even though Broker and Agent
  run in the same binary during M1.
- The instance constructor (`NewExecutionRequest`) always enforces two rules:
  reject an empty Command and normalize empty/zero values (`nil` Arguments and
  a zero `ReceivedAt`).

## Alternatives Considered
1. **Use an external UUID library such as google/uuid** — Rejected in M1 because
   it adds a dependency without a real need. `RequestID` is only an opaque
   string, so a real UUID implementation can replace the internals later
   without changing the public contract.
2. **Omit JSON tags because Broker and Agent remain in one process** — Rejected
   because the project architecture explicitly describes Broker as an
   independent, replaceable adapter (SSH, Web, Console, and others). Separating
   them into distinct processes is plausible in the future.

## Consequences
- Adding a real UUID library later, if necessary, changes only the internal
  `generateRequestID` implementation and not the public function signature.
- Because the contract is JSON-safe from the beginning, a future Broker in a
  separate process (for example, over HTTP or a Unix socket) will not require
  redesigning `ExecutionRequest` or `ExecutionResponse`.
- `Metadata.ClaimedRoles` is deliberately named that way rather than simply
  `Roles`, emphasizing that the value is an unverified claim. ADR-006 clarifies
  that the M5 Policy Engine checks authorization but still does not authenticate it.
