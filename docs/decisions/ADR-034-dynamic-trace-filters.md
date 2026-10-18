# ADR-034: One trace operation with typed conditional filters

Status: Accepted

## Context

Separate host, port and filtered trace operations make completion ambiguous and
force users to select a subcommand before supplying the same named parameters.
A capture must remain scoped to at least one host or port, including for requests
received outside the terminal.

## Decision

Ship only `trace`. Its optional typed host and port parameters have a signed
`required_any` constraint; protocol is an optional allowlisted value. Validation
rejects a request without host or port, and rejects `any` for every input.
Remove the three specialized operations, their aliases and default policy grants.

Introduce signed conditional argument groups: a group names its activation
parameter and whole-token argv template. The translator compiles every group
before serving requests and joins active groups using configured literal tokens.
Defaults participate in activation; the request itself is never mutated. Missing
required-any arguments also fail defensively during translation. This preserves
a shell-free argv boundary and does not admit user-supplied BPF expressions.

The packaged capture uses fixed line-buffered numeric output and capture interface
settings. The new inputs only control the host, port and protocol predicates.
Administrator operation overrides remain authoritative and must be revised if
an installation has explicitly overridden trace or policies referencing removed IDs.

## Validation

Test host-only, port-only, both, each with protocol; reject empty, protocol-only,
empty host, any values, invalid ports, unknown arguments and injection strings.
Test completion of all parameter names and protocol choices and absence of the
removed operations. Test conditional templates, defensive checks, defaults and
immutable compiled snapshots alongside existing integration/security suites.
