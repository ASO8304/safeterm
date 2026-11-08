# Terminal branding and safe navigation

Edit `/etc/safeterm/terminal.json` as a Linux administrator:

```json
{
  "name": "Acme",
  "prompt": "{brand}::{user}{context}{dirty}> ",
  "show_result": false
}
```

The default name is `safeterm`. The same settings apply to Console and SafeTerm
SSH. `{user}` is the authenticated SafeTerm identity. `{context}` includes the
current editor path in parentheses; `{dirty}` is `*` when edits are unsaved.
Keep all three placeholders. `{brand}` is optional. Use printable ASCII text,
at most 64 characters for the name and 160 for the template. Shell expansion,
unknown placeholders and custom ANSI escapes are rejected. Existing theme and
`NO_COLOR` behavior remain supported.

Run `sudo termctl check` to validate. New Console processes load the settings;
restart `safeterm-ssh-broker.service` for new SSH sessions to load them. Package
updates preserve the file.

`show_result` controls the final execution summary, for example
`result: success exit=0` or `result: execution_error exit=1 message=...`.
The default is `false`, including when the setting is omitted in an existing
file. Command output and stderr still appear; unsuccessful commands show a
concise error, and rejected commands retain their rejection reason. Set it to
`true` to restore the detailed summary for troubleshooting. The setting applies
to both ordinary and streaming commands in Console and SSH. Audit logging and
actual exit codes are unchanged. Reopen Console, or restart the SSH Broker and
reconnect, after changing it.

`exit` moves up one level, or closes the session at the main prompt. When that
step would discard unsaved edits, it asks:

```text
Discard unsaved edits and exit this level? [y/N]:
```

Only `y` or `yes` discards. Enter, `n` and Ctrl+C keep the current draft and
context. `next` moves up with edits staged; `save` validates and saves. For
networking, a saved candidate still needs `execute network apply` and timed
`execute network confirm`. `cancel` explicitly discards local drafts, while
Ctrl+] closes the session immediately.
