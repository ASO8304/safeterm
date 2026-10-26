# Ping and TCP port tests

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

## TCP test named telnet

```text
execute telnet 192.0.2.10 443
execute telnet 192.0.2.10
execute telnet ::1 23
execute telnet ?
```

The first argument is a literal unicast IPv4/IPv6 address. DNS hostnames, scoped
IPv6 addresses, unspecified/multicast targets and shell/option strings are not
accepted. Port is 1–65535; omitted port defaults to 23.

This is a TCP connection test, not an interactive Telnet protocol client. It
sends no application data and immediately closes a successful connection:

```text
Trying 192.0.2.10:443...
Connected to 192.0.2.10:443. TCP port is open.
```

Refusal, timeout and unreachable network/host have distinct messages and a failed
execution result. Success confirms the TCP connection only; it does not verify
application health, authenticate or negotiate a Telnet session.

The signed operation has `executor_kind: "tcp-probe"` and
`"tcp_probe": {"timeout_seconds": 5}`. Administrators can override that typed
setting in `operations.d` and run `sudo termctl check` then `sudo termctl apply`.
The supported timeout range is 1–60 seconds. No external telnet/nc package,
additional service account, sudo grant or general stdin channel is introduced.

Ctrl+C cancels an in-progress connection attempt and returns to the SafeTerm
prompt. Ctrl+] retains its global emergency exit behavior. Both Console and SSH
use the same syntax and session-control path.

The probe runs in the existing network Runner, after request validation,
authorization and translation. The native executor accepts only names bound to
signed typed probe definitions and validates the IP/port again before dialing.
All ordinary process operations still pass through the executable-integrity
boundary. Existing session limits and audit handling remain in force.

Regression tests cover argument syntax, count caps, default/custom TCP ports,
request authorization, rejection before dialing, successful connection/close
without data, refusal, bounded timeout, cancellation, completion/help and
continued use of SafeTerm after Ctrl+C. The disposable systemd Console test runs
real loopback ping and TCP probes through the production Runner.
