# REQ-001: Standard ExecutionRequest Contract Between Broker and Agent

**Source:** Project specification, section 7.3 (Broker-Agent communication
contract): unique identifier, command name, structured arguments, and metadata
**Priority:** Critical
**Status:** Done

## Description
Every Broker must convert user input into the standard `ExecutionRequest`
structure before passing it to Agent Core. Agent Core must never accept input
outside this contract. A Broker must not know which real system command maps to
the user's operation; that knowledge belongs only to the Agent's Translation
Layer (M6).

Security boundary (related to ADR-003): values in `Metadata.ClaimedRoles` are
only Broker claims and must not be treated as authenticated identity, even
after Policy Engine (M5). M5 checks authorization for the claimed roles;
authentication is the responsibility of a separate future layer (ADR-006).

## Acceptance Criteria
- [x] **AC-1:** Constructing an `ExecutionRequest` without Command returns an error
- [x] **AC-2:** Every call generates a unique, non-empty `RequestID`
- [x] **AC-3:** A `nil` Arguments value is normalized to an empty map
- [x] **AC-4:** `Metadata.ReceivedAt` is populated automatically when empty
- [x] **AC-5:** `ExecutionRequest` survives a JSON round trip without data loss
- [x] **AC-6:** `ExecutionResponse.Duration()` correctly calculates the interval between start and end
- [x] **AC-7:** `ExecutionResponse.FailureStage` survives JSON transport so
  Audit can identify the rejecting stage without parsing a message

## Related Tests
- TEST-CONTRACT-001 through TEST-CONTRACT-007
  (`internal/contract/request_test.go`, `internal/contract/response_test.go`)

## Related ADRs
- ADR-002 (ExecutionRequest/ExecutionResponse contract design)
