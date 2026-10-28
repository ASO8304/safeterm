# access/policies.d

Administrator JSON definitions. TEMPLATE.json is ALWAYS ignored before opening
or decoding. Copy it to another *.json name to activate a definition. Non-JSON
files and *.json.disabled are ignored. Standard JSON only: no comments.

Use sudo termctl check, then sudo termctl apply from an ordinary Linux admin
shell. All inputs must be root-owned, single-link regular files without group
or world write permission; active symlinks and unknown/duplicate keys fail.

No file is required: verified packaged defaults remain ready to use. A custom
file with an existing name/id replaces that definition; removing the override
restores the packaged default on the next apply. Policies are exact replacements,
not inherited grants. Removing an added operation/profile also removes it at the
next successful apply. User-assigned roles must still exist; users are never
silently changed.

Use termctl defaults TYPE NAME to print a verified packaged definition in the
correct source format, then copy it here and edit it. Types: role, profile,
operation, executable, policy. For a new executable, set its actual SHA-256 with
termctl executable hash /absolute/path. The zero digest in the example is a
placeholder and cannot authorize a real executable. No hashes refresh silently.

See ../README.md and the site-configuration documentation for ordering, isolation
ceilings, recovery, offline signing and examples. Profile definitions select a
packaged base template, never arbitrary systemd text, root users or sockets.
Templates and reference guides refresh on update; custom *.json files survive.
