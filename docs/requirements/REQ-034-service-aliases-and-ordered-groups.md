# REQ-034: Aliased and ordered service control

Restricted users shall control authorized services through logical resource
names and ordered groups without seeing raw physical-unit output.

Acceptance criteria:

1. Existing service.unit definitions retain their public resource-id commands.
   A service may alternatively declare 1..32 unique units in startup order.
   Reject mixed/empty targets, duplicates, invalid units and excessive timeouts.
2. Group start runs forward, stop backward. Group restart finishes all reverse
   stops before any forward starts. Enable/disable do not implicitly start/stop.
3. Every step is an exact signed unit/action pair with explicit group action
   role grants. No user-selected units, shell commands or sudo wildcards exist.
4. A failed/timed-out step or unavailable Audit halts following steps and names
   the alias, action and index. Do not retry or compensate automatically.
5. Bound the entire sequence and each controller call. Preserve completed
   changes and document partial outcomes and systemd-job cancellation limits.
6. Reject overlapping exact-unit sets across active Gateway profiles and within
   a Runner; permit disjoint groups. Do not release reservations on disconnect
   while a bounded dispatched sequence may still execute.
7. User output hides physical unit names and raw systemctl/journal text.
   Protected Audit records attribute real unit/action and step to the initiating
   request and identity before execution and after its result.
8. Keep Linux-style Tab and contextual help, filtered by authorized actions.
   Installed Markdown/plain-text guides include single/group examples and
   ordering, permissions, failure, timeout and recovery semantics.
9. Run unit, authenticated IPC, adversarial and race tests; gate releases on a
   disposable real-systemd/Console test of order, alias output and step audit.
