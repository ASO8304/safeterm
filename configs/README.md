# SafeTerm administrator configuration

SafeTerm includes ready-to-use, signed package defaults. No custom definitions
are needed for normal installation. Add JSON files only when changing a default
or adding a site-specific capability.

```text
/etc/safeterm/
  resources.d/       Service, managed configuration and log resources
  roles.d/           Custom roles and role descriptions
  operations.d/      Command definitions, arguments and optional role grants
  executables.d/     Binary paths, approved SHA-256 hashes and allowed profiles
  profiles.d/        Runner templates and bounded resource/isolation limits
  policies.d/        Exact role-to-operation permissions
  ssh/, console/     Internal credentials
  trust/             Trusted catalog public key
```

Each definition directory contains an always-inactive TEMPLATE.json and English
README.md/README.txt. Copy a template to a new *.json filename before editing.
Only root-owned, non-writable-by-others JSON files are active; duplicate/unknown
properties and invalid references are rejected. No legacy catalog-source loading
or merging of generated data is supported.

For an existing default, emit a correctly formatted override:

```sh
sudo termctl defaults operation ping | sudo tee /etc/safeterm/operations.d/ping.json >/dev/null
sudo nano /etc/safeterm/operations.d/ping.json
sudo termctl check
sudo termctl apply
```

Other default types: role, profile, executable, policy. Built-in values remain
unchanged when no override exists. Removing an override restores the packaged
default; removing an added definition removes it if no references remain.
policies.d entries are exact replacements, without implicit inherited grants.
Use termctl executable hash PATH to obtain a hash; approvals never refresh silently.
Use termctl role/profile/executable list or describe to inspect compiled values.

Runtime data is root-owned under /var/lib/safeterm/generated; never edit it.
Verified package built-ins are under /usr/share/safeterm/defaults; never edit them.
Audit owns only /var/lib/safeterm/audit. The local signer key, when selected,
remains under /root/.config/safeterm.

Profile overrides use fixed packaged isolation templates. A new profile gets a
separate locked system user and generated Runner service/socket through apply.
No arbitrary root identity, systemd text, socket or capability escalation is
accepted. Removed profiles lose their runtime grants; dormant managed accounts
are retained to avoid UID reuse. See the full site-configuration guide in the
repository docs for exact bounds, policy ordering and operational limits.

`sudo termctl apply --rollback` restores a completed definition activation;
`sudo termctl apply --recover` handles interruption. These do not rewrite source
JSON or application values. Package `termctl rollback` is separate and restores
an installation snapshot. Updates preserve custom source files and credentials;
reference templates/guides refresh and generated data/services are rebuilt.
INI comments, whitespace, UTF-8 BOM, ownership and ACLs remain preserved.

This layout extends ADR-032 through ADR-033. Old configurations under direct
operations/roles/profiles/etc directories are refused, never imported or deleted.
Copy only modern source JSON to another host and provision its actual binaries,
files, units, narrow access and signing method separately.
