# REQ-008: Runtime Must Isolate Execution by Capability Profile

**Source:** Product-owner decision to enforce least privilege across operation
types and ADR-009
**Priority:** Critical
**Status:** Planned

## Description
SafeTerm must not deploy one universally privileged Agent. Host execution is
divided into independently supervised Runner services whose boundaries follow
operating-system privilege requirements. A non-executing Node Gateway routes
each approved operation to its configured profile over host-local IPC.

Operations remain declarative and typed so later catalog workflows can let
authorized administrators extend the system without creating an arbitrary
remote-shell interface. M8 establishes the enforceable runtime boundaries and
active catalog schema. Integrity/signing is completed in M10, and remote
proposal/approval workflows are completed with the Broker in M12. Runtime
components enforce their own local allowlists and fail closed independently of
upstream routing.

## Acceptance Criteria
- [ ] **AC-1:** Every operation maps to exactly one configured Runner profile
  and one supported executor kind; missing, unknown, or conflicting mappings
  make configuration loading fail.
- [ ] **AC-2:** The initial runtime supports separate `diagnostics`,
  `network-observer`, and `service-manager` profiles without separate codebases.
- [ ] **AC-3:** Each profile runs as an independent process with its own service
  identity, local IPC endpoint, configuration subset, and resource limits.
- [ ] **AC-4:** A Runner rejects an operation outside its locally loaded profile
  even if the Gateway routes that request incorrectly.
- [ ] **AC-5:** Profiles receive no elevated OS capability by default. Any
  capability grant is explicit, documented, testable, and limited to the
  profile that requires it; production Runners do not run as root by default.
- [ ] **AC-6:** The Node Gateway cannot execute commands, has no Executor in its
  request path, and communicates with Runners only through authenticated
  host-local IPC.
- [ ] **AC-7:** `service-manager` accepts only typed actions against explicitly
  allowlisted service units and never constructs a shell or accepts arbitrary
  `sudo`/`systemctl` arguments.
- [ ] **AC-8:** A process operation can reference only an approved executable ID
  and a tokenized argv template. Callers and catalog editors cannot supply a
  free-form command line or override the selected profile at invocation time.
- [ ] **AC-9:** Active catalog snapshots are schema-validated, versioned, and
  atomically loaded. Invalid snapshots fail closed and never partially replace
  the last valid in-memory configuration.
- [ ] **AC-10:** Gateway and Runner preserve one request ID and expose enough
  structured lifecycle information for M9 Audit to correlate routing, policy,
  and execution decisions.
- [ ] **AC-11:** Integration and negative tests prove that compromise or
  misconfiguration in one routing/profile path cannot invoke an operation from
  another profile through the supported interfaces.

## Related Tests
- Planned: TEST-ISOLATION-001 through TEST-ISOLATION-011

## Related ADRs
- ADR-003 (fixed Agent Core pipeline)
- ADR-007 (tokenized translation without shell construction)
- ADR-008 (bounded direct process execution)
- ADR-009 (capability-isolated services and controlled catalog)
