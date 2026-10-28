# ADR-045: Discoverable terminal help and discard navigation

Status: Accepted

Operators should see their choices after login and when returning to a parent
menu. Asking for contextual help must allow them to continue the command they
have already typed. Returning from an item must have explicit save semantics.

Display the main `?` command menu after successful SSH or Console authentication.
In the command editor, a standalone trailing `?` followed by Enter displays the
existing contextual help and redraws the command prefix without the question
mark. Keep the cursor at the end of that prefix. Help submissions do not enter
history or dispatch execution/mutation. Credential and confirmation prompts are
unaffected; Ctrl+U clears a retained command.

Provide configuration navigation menus with visible prompts, contextual help,
Tab completion and relative choices. Keep direct `config PATH` entry available.
`back` moves up one level and displays that level's available choices.

At the operator's request, `back` discards unsaved edits for the selected network
interface, route or file section. Rebuild a network draft from its original
editor base and the remaining items' changes. File sections retain other
sections' staged changes. Leaving an editor's item list discards its remaining
draft; leaving a logical context discards that context. Navigation never saves
or applies configuration. Existing saved network candidates remain untouched.

`next` retains its existing role of leaving an item while keeping staged edits.
`end` explicitly saves an active editor; `abort`/`cancel` discard the entire
unsaved editor. Help and entry messages explain the distinction.

Both SSH and Console share this implementation. Tests cover login discovery,
retained and editable help input, CRLF, repeated/denied help, absence of side
effects, context/section/item discard boundaries, parent menus and completion.
The disposable systemd Console test exercises the actual terminal and writer.
