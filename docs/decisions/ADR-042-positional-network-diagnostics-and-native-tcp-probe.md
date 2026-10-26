# ADR-042: Positional diagnostics and a bounded native TCP probe

Status: Accepted

## Context

Operators want `ping IP [-c COUNT]` and `telnet IP [PORT]` instead of internal
KEY=VALUE arguments. Telnet is requested solely as a TCP port test, with Ctrl+C
canceling that operation and Ctrl+] retaining global SafeTerm exit behavior.

## Decision

Adapt ping/telnet syntax in the Broker while preserving typed catalog request
fields. Enforce a 1000-request ping ceiling in both the UI and Runner validation.
Offer positional help/completion rather than host=/count= suggestions.

Add the signed operation kind `tcp-probe`, permitted only in the existing network
process-profile family. Its schema requires a host and optional explicitly
bounded/defaulted port plus a 1–60 second timeout. It has no executable path,
command templates, systemd definition or application input. The default operation
is `telnet`, granted to netops, port 23 and timeout 5 seconds.

The network Runner wraps its integrity-checked process delegate with a native
TCP dispatcher. Only signed probe operation names select the native path; all
other commands continue through process integrity checking. The connector parses
IP literals again, uses a deadline/cancellation-aware dial, sends no application
data and always closes the connection. It does not resolve DNS or invoke an
external telnet, nc or shell. Existing streaming session/audit boundaries handle
Ctrl+C cancellation and global emergency exit.

## Consequences

No package dependency or extra daemon is needed. The CLI name telnet is explicitly
documented as a connection test, with no interactive or Telnet negotiation mode.
Default port and timeout can be changed by a signed administrator override;
network access remains subject to operation-role grants and OS isolation.

Unit, Core authorization, session-control and disposable systemd tests cover the
feature. Native TCP operations cannot be loaded by older Runner versions; deploy
the matching package before activating such definitions.
