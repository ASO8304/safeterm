# SafeTerm

A configurable platform for secure system command execution.

## Current Status

**Milestone: M6 — Translation Layer (implemented and verified locally; pending review and release)**

This version does not execute real operating-system commands yet. Available capabilities:

- Request/response contracts and a Core with the fixed Validation → Policy → Translation → Execution sequence
- JSON configuration loading and config-driven argument validation
- Authorization of claimed roles with the real Policy implementation and M5 integration tests
- Translation of operations into fixed config commands and separate arguments, without constructing shell strings

Policy does not authenticate users; `ClaimedRoles` remains an unverified claim (ADR-006).
The real Translation layer was implemented in M6; real Execution belongs to M7. The
`cmd/agent` entry point remains a bootstrap program. Integration tests currently prove
the wiring of the real stages. M6 evidence is recorded in [evidence/M6](evidence/M6/README.md).

Example translation output for trace with port=999 and host=192.168.10.20:

```text
command: tcpdump
args: ["-i", "any", "port", "999", "and", "host", "192.168.10.20"]
```

The `translation.New(cfg)` constructor returns an error that must be checked before
wiring the Translator into Core. A template accepts only a literal token or a complete
`{name}` placeholder, and every placeholder value must be present even when the
parameter is optional during Validation (ADR-007).

## Local Verification

```bash
make verify
```

This command runs the formatting check, `go vet`, build, and unit/integration tests in order.
When every check passes, the final output is:

```text
SafeTerm Verification: PASS
```

## Project Structure

```text
safeterm/
├── cmd/            Executable entry points (agent, broker, ...)
├── internal/       Internal logic (config, validation, policy, ...)
├── configs/        Configuration files (operations, roles, policies)
├── tests/          Integration, security, and end-to-end tests
├── docs/           Requirements, ADRs, and testing documentation
└── evidence/       Successful verification evidence for each milestone
```

## Development Methodology

Every new capability follows this cycle:

```text
Requirement → Design → Implementation → Unit Test →
Integration Test → Security Test → Evidence → Documentation → Git Tag
```

Details are recorded in `docs/decisions/` (Architecture Decision Records) and
`docs/requirements/`.

## Milestone Roadmap

| # | Title | Status |
|---|---|---|
| M0 | Project Foundation | ✅ Tagged: v0.1.0-m0 |
| M1 | ExecutionRequest Contract | ✅ Tagged: v0.2.0-m1 |
| M2 | Agent Core | ✅ Tagged: v0.3.0-m2 |
| M3 | Configuration Loading | ✅ Tagged: v0.4.0-m3 |
| M4 | Validation Engine | ✅ Tagged: v0.5.0-m4 |
| M5 | Policy Engine | ✅ Merged: PR #5; tag pending |
| M6 | Translation Layer | 🔄 Verified locally; pending review/release |
| M7 | Command Runner | ⏳ Planned |
| M8 | Audit Logging | ⏳ Planned |
| M9 | Integrity Verification | ⏳ Planned |
| M10 | Streaming | ⏳ Planned |
| M11 | Brokers (CLI/SSH/Web) | ⏳ Planned |
| M12 | Security Testing | ⏳ Planned |
| M13 | End-to-End Demo | ⏳ Planned |
