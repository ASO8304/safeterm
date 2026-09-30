# M9 Evidence — Audit Service

Date: 2026-09-29. Environment: WSL, Linux/amd64.

## Baseline
- M8 commit `ecfb8d1` passed local verification and GitHub CI in PR #8.
- Branch `codex/m9-audit-service` was created from the reviewed M8 head while
  PR #8 remains pending merge.
- The pre-change `make verify` baseline completed with `SafeTerm Verification: PASS`.

## Implemented Boundaries
- `safeterm-audit` is a separate unprivileged service with no execution path.
- Gateway and every Runner synchronously emit versioned, correlated events.
- Producers and the Audit Service mutually authenticate Unix peers with
  Linux `SO_PEERCRED`; each UID is bound to its component/profile, and messages
  are strict JSON capped at 1 MiB.
- Pre-routing and pre-execution audit failures stop the request. Completion
  failures are returned instead of being silently discarded.
- Records exclude argument values, stdout, stderr, and free-form messages.
- The JSONL store uses mode 0600, fsync before acknowledgement, monotonic
  sequence numbers, and a SHA-256 link to the preceding record.
- Startup verification rejects malformed, incomplete, oversized, modified,
  reordered, internally deleted, and symlink-backed stores.
- Agent Core exposes a structured failure stage without changing its four
  interfaces or fixed pipeline order.

## Verification
[Complete make verify output](make-verify.log) is stored without abbreviation.

```text
SafeTerm Verification: PASS
```

[Race detector output](race.log) covers Audit, Isolation, and integration paths.

## Reproduction

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
make verify
go test -race ./internal/audit ./internal/isolation ./tests/integration -count=1
```

## Limits
- The local hash chain detects corruption within the retained chain but is not
  a signature or an external checkpoint. Complete-file replacement and tail
  truncation require protected remote checkpointing in later control-plane work.
- User IDs and roles remain claims until Broker authentication arrives in M12.
- Retention, export, and reader authorization are deployment concerns beyond
  the M9 local store.
