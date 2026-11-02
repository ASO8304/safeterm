# ADR-050: Netcat TCP and UDP probes

Status: Accepted. Supersedes the native TCP backend in ADR-042.

## Decision

Use the target's approved OpenBSD netcat ELF binary at `/usr/bin/nc.openbsd`
for `execute tcp-test IP [PORT]` and `execute udp-test IP PORT`. Bind it in the
signed executable catalog, hash it on the deployment target and execute the
verified file descriptor through the existing integrity/process boundary.

Typed probe definitions select fixed `-n -v -z -d -w SECONDS` argv, adding `-u`
for UDP and an end-of-options separator before the literal IP and single port.
Reject names, ranges, arbitrary flags, stdin and interactive modes. Keep the
existing TCP default port and signed `telnet` operation ID/alias for permission
compatibility. Add the independent `udp-test` grant to netops.

Probe timeouts are 1–60 seconds plus bounded process cleanup. Preserve streaming
control, process-group cancellation, lifecycle auditing and output limits.
Capture raw netcat diagnostics and report protocol-aware summaries. UDP scan
success does not verify an application response; report inconclusive with exit
code 2 rather than an open-port claim. No Nmap or sudo dependency is added.

## Consequences

Deployment requires `netcat-openbsd`, checked before services are stopped. The
installer can provision it from configured APT repositories or admins can supply
it offline. Older signed TCP catalogs remain readable for upgrade identity and
rollback checks, but a runtime probe requires its approved netcat binding.
