# ADR-009: Capability-Isolated Runner Services and a Controlled Operation Catalog

## Status
Accepted

## Context
SafeTerm must execute operations that need materially different operating-system
privileges. Network capture may need a narrow Linux network capability, service
management needs authority over selected systemd units, and ordinary diagnostics
should need no elevated privilege. Running every operation inside one powerful
Agent would make that process a single privilege-escalation target and would
violate least privilege.

SafeTerm must also let authorized administrators add operations without turning
the product into an arbitrary remote shell. Creating a separate codebase or
service for every command would provide isolation but would make deployment,
upgrades, and configuration unnecessarily complex.

## Decision
- SafeTerm will separate the control plane from host execution. A central Broker
  authenticates callers, applies control-plane policy, selects a target host,
  and routes a signed request. It never executes host commands.
- Each managed host runs an unprivileged Node Gateway. The Gateway validates the
  Broker connection and request envelope, performs no command execution, and
  routes the operation over a local authenticated IPC channel, initially a Unix
  domain socket, to exactly one Runner profile.
- Runner profiles are independent operating-system services separated by
  privilege boundary, not by individual command. They may use the same
  `safeterm-runner` binary but have separate service identities, configuration,
  sockets, capability sets, filesystem access, and resource limits.
- The initial profiles are:
  1. `diagnostics`, with no elevated capability;
  2. `network-observer`, with only the explicitly required network capabilities;
  3. `service-manager`, with authority over an explicit allowlist of service
     units and actions.
- A new profile is justified only when its operations require a different
  privilege, resource, data-access, or trust boundary. SafeTerm will not create
  one service per command.
- Every operation maps to exactly one Runner profile and one typed executor
  kind. The caller cannot select or override the profile. A Runner loads only
  its own operation subset and fails closed when it receives any other operation.
- M7 `execution.Runner` becomes the `process` executor backend. Privileged
  domains may use purpose-built backends. In particular, service management
  uses a typed systemd API with allowlisted unit and action values; it does not
  accept arbitrary `systemctl`, `sudo`, or shell input.
- The operation catalog remains declarative JSON. A process operation references
  an approved executable identifier and an argv token template. It does not
  accept a shell command string. A separate executable registry binds that
  identifier to a canonical absolute path, integrity metadata, and allowed
  profiles.
- Ordinary callers may invoke approved operations. A catalog editor may propose
  a new operation. Activation requires an authorized approver. Registering a new
  executable, widening a profile's privileges, or creating a profile is a
  security-sensitive change with a stronger approval path.
- Active catalog and executable-registry snapshots are versioned, signed,
  schema-validated, auditable, atomically activated, and rollback-capable.
- Broker, Gateway, and Runner independently enforce the parts of policy they
  can verify. A routing error or compromised upstream component must not cause
  a Runner to accept an operation outside its local profile.
- The services remain in one repository unless independent ownership or release
  cadence creates a demonstrated need to split repositories. Each service still
  has its own process, configuration, deployment unit, and security identity.

## Alternatives Considered
1. **One privileged Agent for every operation** — Rejected because compromise
   of one process would expose every configured host capability and defeat
   least privilege.
2. **One service per command** — Rejected because the service count and
   operational overhead would grow with the catalog rather than with meaningful
   security boundaries.
3. **Let users add arbitrary command lines** — Rejected because a free-form
   executable or shell string bypasses the operation allowlist, typed argument
   validation, and review model.
4. **Expose every privileged Runner directly to the network** — Rejected for
   the initial architecture because it multiplies network listeners,
   certificates, and externally reachable privileged components. A local
   Gateway keeps Runner IPC host-local.
5. **Run every Runner as root and rely only on application checks** — Rejected
   because application policy is not a substitute for OS-enforced identities,
   capability bounding, filesystem restrictions, and resource limits.

## Consequences
- A compromised Runner is limited to the privileges and catalog of its profile,
  reducing blast radius compared with a universal Agent.
- The same binary can serve several profiles, keeping implementation and release
  management simpler than separate codebases.
- Configuration becomes a controlled lifecycle rather than an editable remote
  command string. Safe self-service is possible, but activation is distinct
  from proposal.
- Profile routing, local IPC authentication, service hardening, and the active
  catalog schema become explicit work in M8. Signing and executable binding
  are completed with Integrity Verification in M10. Remote proposal, approval,
  and activation workflows are completed with the Broker in M12.
- Audit must correlate Broker, Gateway, and Runner events under one request ID.
- Integrity verification must bind approved executable identifiers to the file
  actually executed, including canonical-path and symlink handling.
- Linux is the first deployment target. Equivalent Windows service identities,
  ACLs, and privilege isolation require a separate platform decision before a
  Windows execution service is claimed as supported.
