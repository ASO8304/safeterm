# REQ-025: External configuration ownership and INI preservation

**Source:** Manage another service's configuration without chown, with security first and comments/layout retained.
**Priority:** Critical
**Status:** Implemented

## Acceptance criteria

- Explicit root-authorized resource opt-in; no root/sudo access for terminal users or Config Managers.
- Separate bounded writer with profile-scoped kernel peer authentication and no arbitrary path/document/command API.
- Revalidate resource, access, field, type, complete document and expected version at the write boundary.
- Preserve UID/GID/mode/ACL/xattrs; reject unsupported security metadata before replacement.
- Root-controlled parent namespace; deny sensitive paths, links, special files and ambiguous metadata.
- Audit before commits; private bounded backups and version-checked restoration.
- INI replacement changes only the selected value token, preserving all other bytes.
- Reject injected newlines/NUL, duplicate sections and unrepresentable quote/comment syntax.
- Root registry check/apply generates writer-only filesystem grants and coordinates active services.
- Verified checkpoints, durable activation recovery, package/catalog lifecycle serialization.
- Integrate binary/unit/group with packages, install/update/uninstall/rollback/completion and preflight.
- Cover root-owned and foreign-owned files with a real non-root IPC client in isolated CI.

## Verification

`internal/configmanager/writer_linux_test.go` tests ACL/xattr/mode retention,
rollback, peer trust, profile/field/preview/version denial, links, audit failure,
and byte-exact 40-section/400-key commented INI edits with LF and CRLF.
`writer_root_linux_test.go` uses root and a kernel UID-65534 child to prove the
manager cannot write the file directly but can perform a registered update while
retaining root/foreign ownership. CI explicitly runs this privileged test binary.
`cmd/termctl/config_manage_linux_test.go` verifies registry activation, service
state, failure recovery and retained journals. Full verify/e2e/security/race
checks remain required.

Related decision: [ADR-029](../decisions/ADR-029-restricted-privileged-configuration-writer.md).
