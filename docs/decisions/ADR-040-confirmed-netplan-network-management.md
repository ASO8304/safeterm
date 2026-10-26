# ADR-040: Typed, confirmed Netplan network management

Status: Accepted

## Context

Ubuntu Server 24.04 installations use different Netplan filenames and may merge several files. Editing a management address can disconnect the operator. Generic INI/JSON configuration editing cannot express Netplan merge semantics, backend activation or timed recovery safely.

## Decision

Expose `config network interface` and `config network route` as reserved typed CLI contexts. Keep Linux Tab completion and contextual `?` help. Persist edits as private candidates; `end` does not activate networking. Reserve `execute network` for status, diff, apply, confirm, rollback and discard.

Discover every effective Netplan source with filename precedence and map editable values to trusted administrator-allowed `/etc/netplan` files. Do not assume a particular basename. Preserve unrelated source bytes and file metadata. Refuse ambiguous device matches, unsupported editable structures and untrusted files. Default interface policy permits no edits; `netops` has default network privileges; additional network-only roles remain available.

Use a dedicated root service with peer-authenticated typed Unix IPC. Console sessions are verified by the identity service; SSH roles are reloaded on each request. The root service never accepts a path, program, shell or YAML supplied by a terminal user. A process lock is acquired before journal recovery. All file and structure inputs are bounded.

Validate a staged tree using Netplan's generator. During `netplan try`, shadow edited `/etc` files with same-basename `/run` files, leaving persistent files intact. Confirmation requires the original actor, a live deadline and unchanged sources/policy. Persist originals atomically before signalling acceptance. Durable recovery journals, independent watchdogs and an explicit persistent-file reapply supplement Netplan's own rollback. Keep only one saved candidate and one last confirmed backup. Refuse lifecycle updates during an unfinished trial.

## Consequences

Network management gains a small privileged boundary, separate policy and OS dependency. A trial is global to the node and cannot run concurrently with another SafeTerm trial. External Netplan/provisioning tools must be coordinated by the administrator. Recovery cannot guarantee connectivity during OS, hardware or backend failure; out-of-band access remains advisable.

This version edits existing networkd Ethernet IP/DNS/MTU and basic routes. It does not provision virtual devices or edit advanced routing, DHCP6, Wi-Fi or NetworkManager configuration. Unsupported structures are preserved or refused rather than guessed.

See [operator guide](../network-configuration.md), REQ-035, and the isolated guest test used to gate package publication.
