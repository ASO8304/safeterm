# ADR-039: Service aliases and ordered groups

Status: Accepted. Extends ADR-033 and the resources.d service model.

## Context

Restricted users need logical service names independent of systemd unit names.
Applications can also consist of an ordered dependency chain. Restarting each
member independently does not stop the complete stack before starting it again.

## Decision

Keep resources.d as the administrator input. A service specifies exactly one
unit or a units array of 1..32 unique exact .service names. Its resource id is the
public alias. The array defines startup order, dependencies first. Start, enable,
reload and status run forward; stop and disable run backward. Group restart
performs all reverse stops before all forward starts. Single-unit restart keeps
the normal systemd behavior. No nested groups or arbitrary shell plans exist.

Compile an immutable typed plan with alias, ordered units, action and optional
timeout into the signed catalog. Translation passes the signed operation ID,
not a physical unit supplied by a caller. Runner resolves only that operation's
exact action and independently validates the plan. sudoers expands the actual
step pairs; restart-only groups get stop/start for those units, not wildcard or
restart/status/enable grants. Existing per-action role checks remain in force.

The total operation timeout defaults to 120 seconds and is bounded to 240;
the profile's controller timeout can impose a shorter per-call limit. Every
step waits for systemctl to return. Failure, cancellation or Audit unavailability
halts the sequence without retries or compensation. Prior successful changes
remain applied. An inactive status is a successful query that reports inactive
or failed. Missing units/controller errors fail closed.

Reserve the complete exact-unit set in the single node Gateway across profiles;
also reject overlapping execution inside each Runner. Conflicts fail without
queuing, disjoint operations may proceed. Gateway retains the reservation across
caller disconnect and, on uncertain transport failure, through the bounded
sequence deadline plus cleanup. Manual systemctl, systemd dependencies/timers,
different aliases of the same physical unit and administrative process restarts
are outside this coordination guarantee. Use canonical unit names consistently.

User responses display only alias, step index/action and summarized state. Raw
systemctl stdout/stderr/errors and journals are not forwarded. Administrator
sources and protected audit records contain physical units; aliases are not an
OS-level secrecy boundary. Before and after each executed step, synchronous
Audit records include actual unit/action, request id, identity and step index.
An unavailable pre-step Audit prevents execution; unavailable post-step Audit
prevents subsequent steps and reports that the completed step may have applied.

## Consequences and limits

Existing service.unit definitions and completion paths remain valid. A group
uses the same execute service ACTION ALIAS command. Configuration saves remain
independent from service control. Source files survive update and rebuild plans.

Ordering must match systemd dependency/readiness configuration; SafeTerm does
not infer a DAG or check application health. A timed-out systemctl client does
not guarantee the submitted systemd job was canceled. Sequences are bounded
best-effort operations, not resumable or atomic transactions. The administrator
uses the Audit step records and systemd to diagnose/recover partial failures.

## Verification

Tests cover ordered expansion, exact grants, role checks, immutable snapshots,
signature-sensitive order, hidden physical names, failures at every step,
timeouts/cancellation, Audit failure gates and cross-profile overlap. An
authenticated Runner IPC test checks durable step attribution. The disposable
systemd/Console smoke test checks real fixture service start/restart/stop order,
alias completion/output and protected audit entries. It must not run on a
developer's normal host.

Service step records deliberately retain the existing V1 event envelope and
accepted/completed outcomes so older binaries can read the chain after package
rollback. Their operation field is a reserved typed child identity:
service-step:PARENT_OPERATION:INDEX:ACTION:UNIT. Input tokens cannot contain the
separator; ParseServiceStep validates/canonicalizes it. Child records share the
initiating request id and identity. Completed child records include ordinary
execution result, exit code and timing fields. No new on-disk event fields or
outcomes are introduced.
