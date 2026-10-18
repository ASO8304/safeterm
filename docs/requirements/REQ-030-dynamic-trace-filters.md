# REQ-030: Dynamic typed trace filters

The packaged restricted terminal must expose one trace operation, without host,
port or filtered subcommands. Host and port are optional individually, but at
least one must have a nonempty value. Protocol is optional and allowlisted.

Acceptance criteria:

1. Host-only, port-only and combined filters execute through the same signed
   trace operation; supplied predicates combine with AND.
2. Empty and protocol-only requests fail before execution with a message stating
   that host or port must be specified, including direct Gateway requests.
3. `any` is invalid for host, port and protocol; typed ranges, unknown argument
   rejection and injection defenses remain active.
4. Tab completes host=, port= and protocol=, including allowed protocol values.
   Already supplied parameters are not repeatedly offered at the next position.
5. The default policy and catalog do not contain trace_host, trace_port or
   trace_filtered; Console and SSH share the same presentation.
6. Conditional argv groups and required-any constraints are declarative signed
   operation fields; no shell or caller-defined BPF expression is introduced.
7. Existing line-buffered live output and cancellation behavior remain intact.

Validation: internal/translation/trace_test.go,
internal/broker/editor_test.go, internal/config/loader_test.go and existing
translation, request validation, integration, deployment and security suites.
