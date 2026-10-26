# REQ-035: Configurable Ubuntu 24.04 network terminal

- Discover effective Netplan source filenames and merges without a hardcoded basename.
- Offer typed Ethernet DHCP4, multiple static IPs, DNS, search domains and MTU editing plus named basic routes/default gateways.
- Preserve existing Linux-style completion and contextual help.
- Separate editor changes, durable candidates and timed activation; require the same actor to confirm.
- Enforce explicit interface/file/role policy at a privileged service boundary, revalidate identities, and keep brokers unprivileged.
- Preserve unrelated configuration/comments and file ownership/mode/ACLs; reject unsafe and ambiguous inputs with actionable errors.
- Reject stale candidates and foreign source/policy edits during confirmation.
- Recover persistent sources and runtime networking after rejected/expired trials or manager restart; bound backup storage.
- Preserve policy/state across updates and refuse package replacement while a trial is pending.
- Gate releases on disposable Ubuntu 24.04 live Netplan tests in a separate network namespace, plus the existing test/security/install checks.

Scope and limits are documented in [the network guide](../network-configuration.md). Design: ADR-040.
