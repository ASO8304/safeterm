# REQ-026: Contextual terminal and configuration drafts

SSH and Console users shall discover authorized commands through contextual
question-mark help and Tab completion, configure registered resources without
repeating coordinates, and save coordinated changes without per-field approvals.

Acceptance criteria:

1. Root discovery presents execute, config, show, whoami, clear and exit.
2. Resource/section context appears in the prompt. Help and completion show only
   authorized resources/operations and registered fields/allowed values.
3. Set stages values, repeated set replaces a pending value, next navigates,
   diff shows redacted unsaved values, end commits and cancel discards.
4. End validates and commits the final batch as one resource transaction with one
   configured backup and activation sequence. Invalid/stale batches never perform
   sequential partial field writes. The draft is retained when saving fails.
5. Secrets use hidden input, never appear in reads/diffs/history, and inline
   secret assignments are rejected. Session termination discards drafts.
6. Batches are bounded, duplicate/mixed/unknown/readonly changes are rejected,
   and the privileged writer independently reconstructs the registered changes.
7. Friendly service paths resolve to existing signed IDs with unchanged role and
   gateway checks. Existing custom operations and legacy script forms still work.
8. Root registry activation and catalog signing remain administrator-only actions
   outside normal restricted-terminal configuration editing.
