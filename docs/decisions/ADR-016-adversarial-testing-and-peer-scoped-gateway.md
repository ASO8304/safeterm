# ADR-016: Adversarial Testing and Peer-Scoped Gateway Authorization

## Status
Accepted

## Context
M0–M13 built validation, RBAC, isolated runners, authenticated Unix IPC,
integrity verification, bounded streaming, an SSH broker, and structured
configuration managers. Most controls already had focused negative tests, but
the repository lacked one explicit threat model, a cross-boundary attack suite,
race and vulnerability gates, and a source invariant preventing later shell
execution from being introduced accidentally.

The M13 typed activation client also exposed a defense-in-depth gap. Gateway
authenticated a Config Manager UID but then offered that peer the same unary
and streaming protocol accepted from the SSH Broker. The shipped managers only
sent registered activation requests, yet a compromised manager process could
construct a different request. OS peer authentication alone established who
connected; it did not constrain what that identity could request.

## Decision
- Maintain `docs/security/threat-model.md` as the architectural security model.
- Add a dedicated `tests/security` suite. It attacks public boundaries and also
  scans production source for forbidden execution patterns. Focused package
  tests remain beside code when private state or platform primitives are needed.
- Treat fuzz functions as a permanent seed corpus in ordinary verification and
  run bounded fuzz campaigns when producing milestone evidence.
- Add `make security`, which runs the adversarial suite and the race detector
  across all packages. GitHub Actions also runs govulncheck, module integrity,
  and CodeQL, retains the SARIF artifact, and runs Dependency Review when that
  repository capability is enabled. These scanners complement tests; they do
  not replace review.
- Extend authenticated Unix Gateway IPC with per-UID `PeerPolicy`. The SSH
  Broker peer may use unary and streaming requests but must present `ssh`
  metadata. Each Config Manager UID is mapped to one profile and may use unary
  requests only. Its command must be an argument-free, bounded systemd
  operation referenced as that profile's activation or health-check operation
  in the root-owned resource registry. The Gateway enforces this before Core.
- Reject managed configuration, audit, and SSH security files with multiple
  hard links on Linux. Symlink rejection alone does not prevent aliasing.
- Bound the Broker's proposal-token routing cache and prune expired routes.
  Config Managers retain their independent proposal bound as the authoritative
  limit.
- Remove the obsolete `cmd/agent` foundation stub and exported M2 bypass
  placeholders. Permissive pipeline doubles remain available only to tests;
  deployed entry points are the Broker, Gateway, profile Runners, Config
  Managers, and Audit Service.

## Alternatives Considered
1. **Only run a penetration test once** — Rejected because findings would not
   become deterministic merge-blocking regressions.
2. **Rely only on CodeQL or a vulnerability database** — Rejected because tools
   cannot prove SafeTerm's identity, routing, token, filesystem, or process
   lifecycle invariants.
3. **Trust all authenticated local peers equally** — Rejected because service
   isolation is intended to limit damage after one component is compromised.
4. **Give Config Managers a second general Gateway client and rely on Policy** —
   Rejected because a compromised peer could forge claimed roles before Policy.
5. **Run indefinite fuzzing in every pull request** — Rejected because CI must
   remain predictable. Seed corpora and bounded milestone campaigns provide a
   stable baseline; longer fuzzing can run on a schedule.

## Consequences
- Gateway startup now requires the root-owned Config Manager registry and an
  explicit `profile=user` mapping for activation-capable local peers.
- A new activation or health operation must be present both in the signed
  execution catalog and root-owned resource registry, and must use the typed
  argument-free systemd backend.
- Security CI uses maintained official GitHub and Go actions and needs network
  access to vulnerability and CodeQL data. Local `make security` remains fully
  useful without those hosted services.
- Passing M14 establishes tested controls for the current threat model, not a
  blanket guarantee against unknown vulnerabilities or a substitute for
  production hardening and independent review.
