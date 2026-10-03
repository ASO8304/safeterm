# REQ-016 — Interactive Terminal and Console Broker

**Source:** M16 — Interactive Terminal UX and Console Access  
**Status:** Implemented in M16

## Requirement

SafeTerm shall provide the same restricted, policy-controlled terminal over its
SSH Broker and a local Linux console login without exposing a general-purpose
shell. Interactive editing must be usable on a normal terminal while every
completion and operation remains limited to the authenticated principal's
catalog and configuration permissions.

## Acceptance criteria

- [x] **AC-1:** The prompt contains the authenticated SafeTerm identity as
  `safeterm::USER> `.
- [x] **AC-2:** Left, Right, Home, End, Delete, Backspace, Ctrl+A, Ctrl+E,
  Ctrl+U, Ctrl+W, and Ctrl+L provide in-line editing.
- [x] **AC-3:** Up and Down traverse bounded, in-memory command history; secret
  values never enter history.
- [x] **AC-4:** Tab completes shell commands, authorized operations and
  parameters, and authorized config resources, sections, and editable fields.
- [x] **AC-5:** SSH continues to reject raw exec, forwarding, subsystems, and a
  general shell while using the improved editor.
- [x] **AC-6:** A dedicated `safeterm-console` login-shell binary uses a
  root-managed OS-user-to-principal mapping and the same Gateway, Runner,
  Config Manager, policy, integrity, streaming, and audit paths as SSH.
- [x] **AC-7:** Gateway and Config Manager IPC authorize the console account by
  peer UID and require `console` metadata, independently of the SSH Broker UID.
- [x] **AC-8:** The installer creates the unprivileged `safeterm` login account,
  assigns only IPC client groups, installs its fixed login shell, and leaves
  password enrollment to the administrator.
- [x] **AC-9:** Preflight requires the console binary, identity mapping, and OS
  account, and the bundle/demo workflow includes console configuration.
- [x] **AC-10:** Unit tests cover editing/history, authorization-filtered
  completion, console identity binding, and broker-type separation.
