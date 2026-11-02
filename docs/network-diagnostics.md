# Ping, TCP and UDP port tests

The restricted terminal exposes these commands to `netops` by default. Other
roles need explicit operation grants. Catalog request fields remain typed `host`,
`count` and `port`; the terminal syntax is positional.

## Ping

```text
execute ping 192.0.2.1
execute ping 192.0.2.1 -c 5
execute ping -c 5 192.0.2.1
execute ping ?
execute ping 192.0.2.1 -c ?
```

An IPv4 literal is required. Omitted `-c` uses the configured default of 1000.
Count must be 1–1000. Values above 1000 are rejected by the Runner's validator,
including direct API requests and a catalog override with a higher numeric max.
The cap counts requests, not successful replies. Existing operation timeout and
session bounds still apply. Output streams live; Ctrl+C stops the operation.

`host=`, `count=`, duplicate `-c` flags, missing option values and extra arguments
are rejected in this CLI. Tab offers `-c` and bounded count suggestions instead of
internal parameter names. Contextual `?` explains the syntax without execution.

## TCP port test

```text
execute tcp-test 192.0.2.10 443
execute tcp-test 192.0.2.10
execute tcp-test ::1 23
execute tcp-test ?
```

The first argument is a literal unicast IPv4/IPv6 address. DNS hostnames, scoped
IPv6 addresses, unspecified/multicast targets and shell/option strings are not
accepted. Port is 1–65535; omitted port defaults to 23.

SafeTerm runs the approved OpenBSD netcat executable with fixed
`-n -v -z -d -w TIMEOUT -- IP PORT` arguments. This makes a TCP connection
without an interactive session or application data and closes it immediately:

```text
Trying 192.0.2.10:443...
Connected to 192.0.2.10:443. TCP port is open.
```

Refusal, timeout and unreachable network/host have distinct messages and a failed
execution result. Success confirms the TCP connection only; it does not verify
application health, authenticate or negotiate a Telnet session.

The signed operation has `executor_kind: "tcp-probe"` and
`"tcp_probe": {"timeout_seconds": 5}`. Administrators can override that typed
setting in `commands/operations.d` and run `sudo termctl check` then `sudo termctl apply`.
The supported timeout range is 1–60 seconds, plus bounded process cleanup.
The OS prerequisite is `netcat-openbsd`, specifically `/usr/bin/nc.openbsd`;
SafeTerm does not use whichever binary the `nc` alternatives link selects.
The package installer checks this dependency before stopping services and can
provision it through `--install-dependencies`, or it can be provisioned offline.
No additional service account, sudo grant or general stdin channel is introduced.

## UDP port test

```text
execute udp-test 192.0.2.10 53
execute udp-test ::1 123
execute udp-test ?
```

Both IP and port are required. The same literal-IP and single-port restrictions
apply. SafeTerm runs fixed netcat `-n -v -z -d -w TIMEOUT -u -- IP PORT`
arguments, with a default timeout of 5 seconds. A signed `udp-probe` definition
uses `udp_probe.timeout_seconds` (1–60). No Nmap dependency is introduced.

Unlike the TCP connection test, netcat's UDP scan sends probe datagrams. A zero
exit status from `nc -zu` does not verify an application response. SafeTerm
therefore reports **inconclusive**, with exit code 2, and suppresses netcat's
potentially misleading “succeeded” message. A failed scan returns a nonzero
result without claiming the port is definitively open or closed. Use a
protocol-specific application test when service health must be verified.
See the [netcat manual caveat](https://man.openbsd.org/nc#CAVEATS).

Ctrl+C cancels an in-progress connection attempt and returns to the SafeTerm
prompt. Ctrl+] retains its global emergency exit behavior. Both Console and SSH
use the same syntax and session-control path.

The probe runs in the existing network Runner, after request validation,
authorization and translation. The typed executor accepts only names bound to
signed probe definitions and validates the IP/port again before constructing
fixed argv. Netcat crosses the same executable-hash and pinned-file-descriptor
integrity gate as other process operations.
All ordinary process operations still pass through the executable-integrity
boundary. Existing session limits and audit handling remain in force.

Regression tests cover argument syntax, count caps, default/custom TCP ports,
request authorization, rejection before dialing, successful netcat connection/close
without data, bounded UDP probes and inconclusive results, refusal, timeout,
cancellation, completion/help and
continued use of SafeTerm after Ctrl+C. The disposable systemd Console test runs
real loopback ping, TCP and UDP probes through the production Runner.

`execute telnet IP [PORT]` remains a deprecated alias. It prints
`Use 'execute tcp-test' for port probing`, then runs the same authorized probe.
The signed catalog operation ID remains `telnet` for existing permissions.
