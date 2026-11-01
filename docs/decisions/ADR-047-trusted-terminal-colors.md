# ADR-047: Trusted terminal colors

Status: Accepted

Use a small semantic ANSI palette for interactive SSH and Console presentation.
Distinguish prompt branding, user identity and configuration path. Reuse the
palette for commands, headings, status values, drafts, errors and local diffs.
Retain textual labels and symbols so color is never the only status indicator.

Select color support before Run from the accepted SSH PTY terminal type or the
Console TERM value. Keep plain output for unset/dumb terminals, a nonempty
NO_COLOR in the process environment, and embedded callers without SetTerminal.
Do not accept arbitrary SSH environment requests to configure presentation.

Keep ANSI emission separate from data filtering. Style helpers sanitize text in
both modes and emit only known constant SGR codes. External command data cannot
request styles or bypass filtering. Measure network table columns as plain text,
then apply styles without changing padding. Reset styles before typed input.

Coloring does not change authorization, editor state, application writes or
network candidate/apply semantics. Tests verify input editing, hidden secrets,
contextual help, table layout, escape filtering and SSH PTY negotiation.
