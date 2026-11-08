# ADR-055: Self-service password boundary and guided CLI

Status: Accepted

`passwd` (hidden alias `change_pass`) replaces root-level `whoami`. It asks for
current/new/confirmation secrets without echo, completion or history, and cannot
accept a target account or password in command arguments. Existing staged
logical configuration must be saved or cancelled first.

A separate root password manager owns the narrow self-service write boundary.
The Console and SSH Broker remain unprivileged, and Identity remains read-only.
The private Unix API checks kernel peer UID/PID, bounds frames, connections and
attempts, and verifies Console session tokens with Identity. Only the configured
SSH Broker UID may claim an SSH identity. Mapped Console requests additionally
resolve the root-managed mapping to the same kernel UID. Shared Console requests
can change only the authenticated internal account. Root and service accounts
cannot be Linux Console password targets.

SSH and legacy Console passwords use separate, fixed root-owned JSON paths.
Current bcrypt passwords must match before generating a replacement. Atomic
writes preserve owners, modes, roles, keys and unrelated records; unsafe files,
links, ACLs and untrusted parents fail closed. Password authentication reloads the
SSH database without restarting the broker. Legacy Console sessions refresh
their token after their credential fingerprint changes.

Mapped Linux Console changes use only the trusted installed password-manager
child and `/usr/bin/passwd`, without a shell or caller-selected executable.
The child is root-only, re-resolves the target, drops supplementary groups and
sets the real UID to the operator while retaining its existing effective UID.
System passwd/PAM therefore verifies the operator's current password, with
NoNewPrivileges still enabled and no setuid privilege gain at exec. Secrets
travel over stdin, never argv, files, audit or captured output. The helper has a
15-second deadline. Linux password integration is exercised only in disposable
CI, including wrong-current-password rejection under the deployed service.

The root service retains the capabilities needed for ownership preservation,
UID/GID transitions, shadow access and PAM audit, with no ambient capabilities.
Its writable `/etc` boundary permits the system password tool's atomic account
file updates; the public API supplies no paths. It has finite process/memory/FD
limits, no core dumps, a fixed environment and restricted address families.
Changes require a durable pre-write audit event under the existing root
file-writer policy. Completion audit/durability failures after a committed write
report that the password changed and require signing in again. A missing password
service does not prevent unrelated login, command or configuration use.

The root mapping does not remove existing OS privileges: restricted operator
accounts still need the fixed Console login shell and no unrelated sudo, group,
file-transfer or privileged file access. Linux administrators remain trusted.

Routes use guided add/edit with validated destination, gateway, interface and
metric defaults. Cancelling leaves the original editor unchanged; completing
the guide stages changes only. IDs, default-route deletion protection, manual
set commands and save/apply/confirm remain enforced. `direct` and `auto` clear
the optional gateway and metric.

Capture accepts `host VALUE port VALUE protocol VALUE`, while hidden legacy
KEY=VALUE syntax remains compatible. Signed parameter validation and required
host-or-port semantics are unchanged. Typed `?` retains contextual help but is
omitted from advertised help and Tab. Quiet result presentation distinguishes
accepted Ctrl+C cancellation from failure and avoids repeating typed probe
diagnoses; debug summaries and underlying response/audit statuses are retained.
