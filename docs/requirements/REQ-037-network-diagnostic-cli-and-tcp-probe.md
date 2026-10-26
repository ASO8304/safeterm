# REQ-037: Standard positional network diagnostic syntax

- Accept `execute ping IP [-c COUNT]` with -c before/after IP, default/max 1000,
  bounded count validation in the Runner and live output.
- Remove host=/count= from the ping CLI and completion; retain typed backend fields.
- Accept `execute telnet IP [PORT]` as a single TCP port test, default port 23 and
  signed configurable timeout 5 seconds (allowed range 1–60).
- Use literal IPs only, no application data, shell, external telnet/nc dependency,
  general stdin channel or additional privilege grants.
- Ctrl+C cancels only the operation; Ctrl+] exits SafeTerm. Keep contextual help
  and Linux-style completion.
- Verify successful close, refusal, timeout, invalid/unauthorized requests and
  cancellation; exercise real operations in the disposable systemd test.

Design: ADR-042. Guide: [network diagnostics](../network-diagnostics.md).
