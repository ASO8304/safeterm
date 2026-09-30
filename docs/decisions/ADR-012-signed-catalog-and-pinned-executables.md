# ADR-012: Detached Catalog Signatures and Pinned Executable Files

## Status
Accepted

## Context
M8 binds operations to absolute executable paths, but an absolute path alone
does not prove which bytes will run. A file can be replaced after catalog
approval, a symlink can be retargeted, or a compromised catalog can authorize a
different path and digest. Checking a pathname and then reopening it for
execution also leaves a path-substitution window between verification and
`execve`.

M10 needs a standard-library design that keeps private signing material out of
Gateway and Runner services, preserves the existing Agent Core interfaces, and
works with the Linux-first runtime established by ADR-009 and ADR-010.

## Decision
- Each executable registry entry contains an exact lowercase SHA-256 digest.
  A systemd profile names its fixed controller through
  `controller_executable_ref`, so `/usr/bin/systemctl` has the same integrity
  boundary as process operations.
- Gateway and Runner require a detached Ed25519 signature before using a
  catalog. The public trust key is provisioned separately from editable catalog
  files. Runtime services receive only the public key and signature; signing
  private keys remain offline.
- Runtime stores retain the verifier used at startup. Every later atomic
  activation must pass that verifier before it can replace the current
  snapshot. The previous verified snapshot is retained for atomic rollback;
  rollback never introduces new external bytes.
- Signatures cover a canonical JSON representation of the validated Config.
  Top-level definitions and other semantically unordered lists are sorted on a
  deep copy. Argument-template token order remains intact because it changes
  execution semantics.
- `cmd/catalog-sign` uses only the Go standard library to generate Ed25519 key
  pairs and create detached signatures. Private-key files are mode 0600 and key
  generation uses exclusive creation rather than overwriting existing files.
- A profile-scoped integrity Executor builds an allowlist from its filtered
  catalog. At startup and immediately before every call, it resolves symlinks,
  requires the same canonical path, opens the file, requires a regular native
  ELF executable with at least one execute bit and no group/world write bit,
  and checks the full SHA-256 digest.
- After verification, the Executor keeps the file open and passes
  `/proc/self/fd/<fd>` to the existing process Runner. The child therefore
  selects the verified open inode rather than reopening the catalog path. The
  descriptor remains open until execution completes.
- Integrity remains the Executor boundary. A mismatch returns exit code -1 and
  no output, and Core reports the existing structured execution-stage failure.
  Validation, Policy, Translation, and Core are unchanged.
- Production service hardening keeps executable files on read-only system
  mounts (`ProtectSystem=strict`). Hashing and descriptor pinning do not claim
  to defeat a writer that can modify the same inode or the running kernel; such
  authority is outside the Runner trust boundary.

## Alternatives Considered
1. **Trust absolute paths and filesystem permissions only** — Rejected because
   it cannot detect authorized-path content replacement or catalog tampering.
2. **Verify SHA-256 once at service startup** — Rejected because files can
   change while a long-running Runner remains active.
3. **Hash a path and then execute the same path** — Rejected because a rename or
   symlink retarget can substitute a different inode after verification.
4. **Store the signing public key inside the signed catalog** — Rejected because
   an attacker could replace both the catalog and its self-declared trust key.
5. **Use HMAC for catalog signatures** — Rejected because every verifier would
   need the signing secret and a compromised runtime could then forge catalogs.
6. **Add a third-party signing or syscall package** — Rejected because Ed25519,
   SHA-256, canonical encoding, and the required file handling are available in
   the Go standard library.
7. **Allow scripts with shebang interpreters** — Rejected for M10 because the
   interpreter would become a second executable outside the signed binding.

## Consequences
- Catalog changes, including executable hash updates, require a new valid
  signature before Gateway or Runner restart/activation.
- Executable digests are deployment-specific. Operators must hash the exact
  target-host binaries, update the registry, and sign that catalog with their
  offline key.
- Linux `/proc` and native ELF binaries are explicit runtime requirements for
  M10 execution pinning. Non-Linux execution fails closed until an equivalent
  platform-specific design is accepted.
- Replacing or retargeting an executable after startup is detected on the next
  request without relying on the Gateway or Broker.
- The shipped public key and signature are development evidence, not a shared
  production signing identity. Each deployment generates and protects its own
  offline key pair.
