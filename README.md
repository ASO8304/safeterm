# SafeTerm

A configurable platform for secure system command execution.

## Current Status

**Milestone: M8 — Capability-Isolated Runtime (implemented and verified locally; pending review and release)**

Available capabilities:

- Request/response contracts and a Core with the fixed Validation → Policy → Translation → Execution sequence
- JSON configuration loading and config-driven argument validation
- Authorization of claimed roles with the real Policy implementation and M5 integration tests
- Translation of operations into fixed config commands and separate arguments, without constructing shell strings
- Bounded process execution with separate argv elements, independent stdout/stderr capture, and exit-code reporting
- Independently deployable Gateway and capability-isolated Runner services with authenticated Unix IPC
- Declarative profile/executor mappings, an executable registry, atomic catalog snapshots, and typed systemd operations

Policy does not authenticate users; `ClaimedRoles` remains an unverified claim (ADR-006).
The Agent Core pipeline and capability-isolated host runtime are implemented
through M8. The remote Broker and authenticated user identity arrive in M12;
the current Node Gateway accepts only authenticated local IPC. M8 evidence is
recorded in [evidence/M8](evidence/M8/README.md).

Example translation output for trace with port=999 and host=192.168.10.20:

```text
command: /usr/bin/tcpdump
args: ["-i", "any", "port", "999", "and", "host", "192.168.10.20"]
```

The `translation.New(cfg)` constructor returns an error that must be checked before
wiring the Translator into Core. A template accepts only a literal token or a complete
`{name}` placeholder, and every placeholder value must be present even when the
parameter is optional during Validation (ADR-007).

`execution.New(timeout)` requires a positive timeout. Runner invokes
`exec.CommandContext(ctx, command, args...)` directly, never through a shell,
and returns `execution.ExitCodeUnavailable` (`-1`) when no portable process
exit status exists (ADR-008).

## Target Service Architecture

SafeTerm will not deploy one universally privileged Agent. The M7 Runner is an
execution backend that will run inside capability-isolated service profiles.
A central Broker authenticates and routes requests to an unprivileged Node
Gateway; the Gateway performs no execution and forwards each operation over
host-local IPC to exactly one Runner profile.

```text
Client -> Broker -> Node Gateway -> diagnostics Runner
                                -> network-observer Runner
                                -> service-manager Runner
```

Profiles share a codebase but run as separate processes with separate service
identities, configuration subsets, sockets, OS capabilities, filesystem access,
and resource limits. New profiles follow privilege boundaries rather than
individual commands. ADR-009 and REQ-008 define this target and its acceptance
criteria.

Users invoke approved operations, not raw command lines. Authorized catalog
editors may propose declarative operations that reference an approved executor,
profile, and tokenized argument template. Activation is a separate authorized
step. New executables or wider privileges require stronger security approval.

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
├── cmd/            Executable entry points (gateway, runner, ...)
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
| M5 | Policy Engine | ✅ Tagged: v0.6.0-m5 |
| M6 | Translation Layer | ✅ Tagged: v0.7.0-m6 |
| M7 | Command Runner | ✅ Tagged: v0.8.0-m7 |
| M8 | Capability-Isolated Runtime | 🔄 Verified locally; pending review/release |
| M9 | Audit Service | ⏳ Planned |
| M10 | Integrity Verification | ⏳ Planned |
| M11 | Streaming | ⏳ Planned |
| M12 | Broker, Authentication, Catalog Workflow, and Client Adapters | ⏳ Planned |
| M13 | Security Testing | ⏳ Planned |
| M14 | End-to-End Deployment and Demo | ⏳ Planned |
