# ADR-010: Authenticated Local IPC and Typed Execution Backends

## Status
Accepted

## Context
ADR-009 separates the Node Gateway and capability-specific Runners into
different operating-system processes. M8 therefore needs a host-local transport
that preserves request correlation, authenticates both peers, and does not add
a remotely reachable listener. It also needs to support service management
without turning `systemctl` arguments into caller-controlled command input.

## Decision
- Gateway-to-Runner and initial local-Broker-to-Gateway communication uses Unix
  domain sockets with JSON request/response framing.
- Both client and server verify Linux `SO_PEERCRED` UIDs. Filesystem ownership
  and socket mode provide an additional boundary but are not the sole identity
  check.
- Each connection carries one bounded message and one response. Decoding is
  capped at 1 MiB. Streaming and multiplexing are deferred to M11.
- The Gateway depends only on a transport `Client`; it does not import or hold
  an `agent.Executor` and cannot fall back to local command execution.
- A Runner uses `ProfileHandler` as a final local allowlist before Agent Core.
  Upstream routing success is never treated as authorization for another profile.
- Process operations resolve a catalog `executable_ref` to an absolute path.
  ADR-012/M10 verifies its canonical path and file hash immediately before execution.
- Service operations compile to an exact unit/action pair. `SystemdExecutor`
  checks that pair against its catalog snapshot before invoking its controller.
  The initial Linux controller executes a fixed absolute `systemctl` adapter
  with exactly two argv elements; no caller can add flags, change the unit, or
  select a shell. Deployment must separately grant the service account only the
  cataloged systemd actions and units.
- Active catalogs are cloned and validated before an atomic pointer swap. A
  rejected update cannot partially mutate the current snapshot.
- Gateway and Runner expose transport-neutral lifecycle events containing the
  request ID, catalog version, component, profile, operation, outcome, and
  timestamp. Arguments are excluded. M9 will persist these events.

## Alternatives Considered
1. **Local TCP with bearer tokens** — Rejected because it creates a network
   listener and secret lifecycle where kernel-provided local peer credentials
   are sufficient for the Linux-first runtime.
2. **Trust socket permissions without checking peer credentials** — Rejected
   because explicit UID verification is a cheap independent control.
3. **Let Gateway execute when a Runner is unavailable** — Rejected because a
   fallback Executor would collapse the privilege boundary.
4. **Pass arbitrary `systemctl` argv through the process Runner** — Rejected
   because flags, units, and actions would escape the typed catalog model.
5. **Add a D-Bus dependency in M8** — Deferred because the standard-library
   adapter preserves the typed boundary without adding a dependency. A future
   native D-Bus adapter may replace the controller behind the same interface
   under a separate dependency decision.

## Consequences
- M8 is Linux-first; non-Linux builds fail closed when authenticated Unix peer
  credentials are unavailable.
- Local IPC is deliberately request/response only until M11 introduces a
  bounded streaming protocol.
- Deployments must provision separate users and a narrow OS authorization rule
  before activating service-manager operations.
- ADR-012 completes executable integrity in M10: the systemctl adapter and
  every process executable are canonicalized, hash-verified, and descriptor-
  pinned immediately before execution.
