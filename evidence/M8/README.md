# M8 Evidence — Capability-Isolated Runtime

Date: 2026-09-27. Environment: WSL, Linux/amd64.

## Baseline
- PR #7 was squash-merged as commit `044dd9e` after both CI runs passed.
- Annotated tag `v0.8.0-m7` points to the merged M7 commit and was pushed to origin.
- Branch `codex/m8-capability-runtime` was created from `origin/main` at `044dd9e`.

## Implemented Boundaries
- Operations declare one Runner profile and one typed executor kind.
- Process operations reference an executable registry ID that resolves to an
  absolute path; callers cannot provide an executable or raw command line.
- The initial profiles are `diagnostics`, `network-observer`, and
  `service-manager`.
- `safeterm-gateway` has no Executor. It routes requests to profile clients.
- `safeterm-runner` is one binary deployed as separate OS services, users,
  sockets, resource limits, and capability sets.
- Gateway and Runner authenticate both ends of Unix socket communication with
  Linux `SO_PEERCRED`; messages are capped at 1 MiB.
- A ProfileHandler rejects cross-profile operations before Agent Core.
- Systemd operations compile to fixed unit/action pairs and reject arbitrary
  units, flags, actions, parameters, `sudo`, and shell input.
- Catalog snapshots are cloned and validated before atomic activation. Failed
  activation preserves the previous snapshot.
- Lifecycle events expose request ID, catalog version, component, profile,
  operation, outcome, and timestamp without argument values for M9 Audit.

## Verification
[Complete make verify output](make-verify.log) is stored without abbreviation.

```text
SafeTerm Verification: PASS
```

[Race detector output](race.log) covers configuration, execution, isolation,
and integration packages.

## Reproduction

```sh
cd /mnt/c/Users/abolf/OneDrive/Documents/safeterm
GIT_CONFIG_COUNT=1 \
GIT_CONFIG_KEY_0=safe.directory \
GIT_CONFIG_VALUE_0=/mnt/c/Users/abolf/OneDrive/Documents/safeterm \
make verify

go test -race ./internal/config ./internal/execution ./internal/isolation ./tests/integration -count=1
```

## Deployment Notes and Limits
- Linux is the first execution target. Non-Linux peer authentication fails closed.
- The service-manager catalog ships with no active unit operation. A deployment
  must provision a narrow OS authorization rule before enabling one; unrestricted
  sudo or general systemd management is forbidden.
- M10 adds canonical-path and executable hash verification.
- M11 replaces one-request IPC with bounded streaming for long-running operations.
- M12 adds the remote Broker, user authentication, catalog proposal/approval,
  signing, and client adapters.
