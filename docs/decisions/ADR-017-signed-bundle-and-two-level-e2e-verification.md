# ADR-017: Signed Bundles and Two-Level End-to-End Verification

## Status
Accepted

## Context
M8–M14 defined a multi-service system whose security depends on OS identities,
Unix peer credentials, systemd sandboxing, a detached catalog signature,
target-host executable hashes, SSH keys, and root-owned resource declarations.
Individual units and integration tests existed, but operators had to assemble
these artifacts manually. A configuration copied from another host could also
contain executable hashes that did not match the deployment host.

A full service demo needs root, systemd, dedicated accounts, tcpdump, and a
free TCP port. Requiring those privileges in every CI runner would weaken test
isolation and make ordinary verification unreliable. Conversely, a test made
only from mocks would not prove the real SSH, Unix IPC, integrity, process, and
audit path.

## Decision
- Use a two-phase catalog workflow. `catalog-prepare` runs against the target
  host's executable paths and creates a new unsigned catalog with refreshed
  hashes. An operator transfers that catalog to an offline signer and returns
  only the public key and detached signature. No runtime bundle contains the
  private catalog key.
- Package binaries, the signed configuration tree, managed seed files, and
  systemd units in one immutable directory with a SHA-256 manifest. Reject
  symlinks and verify both the manifest and semantic preflight before host
  mutation.
- Keep installation explicit and root-only. The script provisions the fixed
  service identities and groups, installs restrictive modes, validates the
  installed state, starts services in dependency order, and checks that all
  units are active. `--dry-run` performs bundle verification without mutation.
- Provide two complementary E2E levels. The normal Go suite runs a real SSH
  server/client, Unix sockets, Gateway, Runner, verified native process, and
  durable Audit store under one test UID. The disposable-host demo runs the
  separately built systemd services with their real UIDs and capabilities.
- Scope the installer to fresh dedicated hosts. It refuses incomplete input;
  general upgrades and removal are deferred until package ownership and
  migration semantics are designed.

## Alternatives Considered
1. **A container containing systemd for every CI run** — Rejected because it
   requires privileged runners and still does not faithfully model the target
   host's users, cgroups, capabilities, and system binaries.
2. **Generate and retain the catalog signing key on the runtime host** —
   Rejected because it collapses the offline trust boundary established in
   ADR-012. The demo uses a disposable key only and deletes it with its temporary
   workspace.
3. **Install directly from the source checkout** — Rejected because source,
   build tools, mutable configs, and signing material do not belong on a
   runtime host. The verified bundle is the installation boundary.
4. **Use only in-process mocks for the demo** — Rejected because mocks cannot
   establish that SSH framing, peer credentials, pinned executable descriptors,
   process execution, and durable audit all interoperate.

## Consequences
- Production deployment requires a target-host digest collection step followed
  by offline approval/signing before bundle creation.
- CI proves the complete data path but not systemd's enforcement; the root-only
  disposable-host demo covers service identities, unit ordering, capabilities,
  and filesystem permissions.
- Fresh installation is reproducible and reviewable. Upgrade, rollback, native
  OS packages, port customization, and external secret management need future
  requirements rather than implicit installer behavior.
