# Terminal branding and safe navigation

Edit `/etc/safeterm/terminal.json` as a Linux administrator:

```json
{
  "name": "Acme",
  "prompt": "{brand}::{user}{context}{dirty}> "
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
