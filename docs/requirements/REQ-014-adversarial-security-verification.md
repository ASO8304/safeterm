# REQ-014: Security Controls Must Survive Adversarial Verification

**Source:** M14 — Security Testing and ADR-016
**Priority:** Critical
**Status:** Implemented in M14

## Description
SafeTerm must verify its security properties across the complete deployed
architecture rather than relying only on happy-path unit tests. The suite
models untrusted SSH input, forged identity metadata, malformed local IPC,
catalog and filesystem replacement, configuration patch tampering, secret
exposure, and CPU/memory/process/output exhaustion. A security failure must be
reproducible, fixed in the owning boundary, and retained as a regression test.

Security testing does not claim that a passing automated suite proves the
absence of vulnerabilities. It supplies repeatable evidence for the documented
threat model and makes the most important invariants merge-blocking.

## Acceptance Criteria
- [x] **AC-1:** A maintained threat model identifies assets, trust boundaries,
  attacker capabilities, security invariants, exclusions, and the control and
  test that mitigates each material threat.
- [x] **AC-2:** An injection corpus covers separators, substitution, quoting,
  redirection, control characters, whitespace, and oversized values. Invalid
  requests are rejected before translation or execution.
- [x] **AC-3:** A source-level invariant test fails if production code imports
  `os/exec` outside the bounded execution package or introduces a known shell
  interpreter. Configured execution continues to use fixed executable IDs and
  separate argv elements.
- [x] **AC-4:** Authentication and privilege-abuse tests cover forged roles,
  cross-user session control, cross-profile routing, Config Manager approval,
  and OS-peer-specific Gateway permissions. A Config Manager peer can invoke
  only its registered, argument-free typed activation operations and cannot use
  streaming.
- [x] **AC-5:** Local IPC rejects untrusted peers, oversized frames, unknown
  fields, trailing documents, ambiguous request shapes, raw signal controls,
  and unsupported protocol versions before reaching a handler.
- [x] **AC-6:** Filesystem tests cover traversal, symlink replacement, hard-link
  aliasing, non-regular files, insecure modes, executable hash changes, and
  path changes between validation and use.
- [x] **AC-7:** Catalog signature, executable digest, native executable format,
  and pinned-descriptor checks reject modified or interpreter-backed content.
- [x] **AC-8:** Runtime resource tests cover output limits, backpressure,
  concurrent session ceilings, idle/execution deadlines, bounded IPC, bounded
  proposal routing, and process-group cleanup.
- [x] **AC-9:** Audit and configuration tests prove that secrets and argument
  values are excluded, terminal control bytes are neutralized, audit producer
  sources cannot be spoofed, and hash-chain corruption is detected.
- [x] **AC-10:** Config Manager tests cover token tampering, replay, expiry,
  actor binding, separate approval, stale versions, cross-profile access,
  parser ambiguity, failed activation rollback, and atomic-write recovery.
- [x] **AC-11:** Fuzz seed suites exercise request validation, contract decoding,
  registry validation, and stream-frame validation without panics or bypasses.
- [x] **AC-12:** The race detector passes across all packages, including
  concurrent audit, streaming, proposal, and session state.
- [x] **AC-13:** CI runs the adversarial/race suite, Go vulnerability analysis,
  dependency integrity verification, and CodeQL with least-privilege workflow
  permissions. GitHub Dependency Review additionally runs when the repository
  enables that platform capability.
- [x] **AC-14:** M14 stores local verification, race, fuzz-seed, and security
  scan evidence and maps every acceptance criterion into the test matrix.

## Related Tests
- TEST-SECURITY-001 through TEST-SECURITY-014
- Existing M10–M13 security and integration regression suites

## Related ADRs
- ADR-005, ADR-006, ADR-009 through ADR-016
