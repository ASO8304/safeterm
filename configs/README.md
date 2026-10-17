# SafeTerm Catalog Administration Guide

## 1. Purpose and scope

The catalog defines the operations SafeTerm exposes, their argument rules,
which roles may invoke them, and the isolated execution backend for each one.
It is an allowlist, not a general-purpose shell configuration.

Example request path:

    execute ping host=192.168.1.10 count=5
      -> validate operation and parameter values
      -> authorize the authenticated user's roles
      -> translate approved tokens to a fixed executable and argv
      -> route to the configured Runner profile
      -> verify the executable's integrity and execute it
      -> return output and record audit events

The SSH Broker authenticates users. Policy authorizes their server-owned roles.
A role is a permission group; a Runner profile is an operating-system isolation
boundary. Neither is the Linux service account used to run the SSH Broker.

## 2. Locations and file layout

In a source checkout, this directory is configs/. After installation, the
runtime configuration is under /etc/safeterm. Read this guide with:

    less /etc/safeterm/README.txt
    cat /etc/safeterm/README.txt

The two guide formats contain the same information. README.md is Markdown;
README.txt is plain UTF-8 text with no terminal escape sequences.

    catalog.json                       Catalog version metadata
    operations/operations.json         Operation definitions
    roles/roles.json                   Role names and descriptions
    policies/policies.json             Role-to-operation permissions
    profiles/profiles.json             Isolated Runner definitions
    executables/executables.json       Approved binaries and SHA-256 digests
    catalog.sig                        Detached Ed25519 catalog signature
    trust/catalog-ed25519.pub           Catalog verification public key
    */TEMPLATE.jsonc                    Commented authoring examples

The loader reads every file ending in .json within the five catalog sections.
Each section file contains a JSON array; catalog.json is a JSON object.
Do not save backup files ending in .json in those directories: they would be
loaded too. Markdown, plain text and .jsonc examples are not loaded.

The installer also creates these adjacent, separately managed files:

    ssh/users.json                     SSH identities, roles and authentication
    ssh/ssh_host_ed25519_key            SSH server identity (private key)
    console/users.json                 Internal Console users, roles and bcrypt hashes
    config-manager/resources.json      Registered editable resources and fields

These adjacent files are not part of the signed catalog model. They must still
be protected by root ownership and their required file permissions. Managed
application settings live under /etc/safeterm-managed; audit records live
under /var/lib/safeterm. Never publish identity files or private keys.

## 3. Catalog metadata: catalog.json

    { "version": "1.2.0" }

The version identifies catalog content. It is independent of the SafeTerm
software release tag. Changing the version is a signed catalog change; a
version string alone does not prove authenticity or enforce rollback protection.

## 4. Operations: what the user can request

Each operation has these fields:

- name: unique public operation name used after execute.
- description: text displayed to operators.
- runner_profile: the profile responsible for execution.
- executor_kind: process or systemd; must agree with the selected profile.
- executable_ref: approved executable ID for process operations.
- args_template: ordered argv tokens for a process operation.
- parameters: accepted input names, types and restrictions.
- long_running: whether the operation uses the bounded streaming path.
- systemd: fixed unit and action for a typed systemd operation.

A process example:

    {
      "name": "system_info",
      "description": "Show basic host information.",
      "runner_profile": "diagnostics",
      "executor_kind": "process",
      "executable_ref": "uname",
      "args_template": ["-a"],
      "parameters": [],
      "long_running": false
    }

The executable path comes from the registry, never from user input. Templates
use literal tokens or complete placeholders such as {host}; embedded shell
expressions and concatenated placeholders are not a shell language. SafeTerm
passes separate argv elements without constructing a shell command string.

Parameter fields:

- name: canonical input name.
- type: int or string (these are the supported operation parameter types).
- required: whether the caller must provide a value.
- default: optional value represented as a string, including integer defaults.
- aliases: alternative input names, such as -c for count.
- min and max: integer boundaries for int parameters.
- pattern: regular-expression restriction for string parameters.
- value_map: public aliases mapped to fixed catalog-owned argv values.

Example ping count definition:

    {
      "name": "count", "type": "int", "required": false,
      "default": "100", "aliases": ["-c"], "min": 1, "max": 1000
    }

In the restricted terminal:

    execute ping host=127.0.0.1
    execute ping host=127.0.0.1 count=5
    execute ping host=127.0.0.1 -c=5

The explicit count replaces the default. The default is bounded, not unlimited.
Operation timing and output limits still apply. Use describe ping for the
installed parameter contract and Tab for completion. A parameter used in an
argv placeholder must have a value after validation/default handling.

A value_map can expose a log alias such as service_A while translating it to
/var/log/safeterm-readable/service_A.log. Users choose approved aliases rather
than arbitrary filesystem paths. Network capture operations similarly require
the filter inputs defined by their own contracts: trace_host, trace_port,
trace and trace_filtered do not share an unrestricted tcpdump command line.

A typed systemd example:

    {
      "name": "status_safeterm_gateway",
      "description": "Show Gateway status.",
      "runner_profile": "service-manager",
      "executor_kind": "systemd",
      "systemd": { "unit": "safeterm-gateway.service", "action": "status" },
      "parameters": [], "long_running": false
    }

The unit and action are fixed. A restricted-terminal user cannot pass an
arbitrary service name to systemctl. Local administrator commands such as
sudo termctl service restart all are a separate OS administration interface.

## 5. Roles: permission group names

roles/ defines known role names and descriptions, not executable paths or
Linux privileges. Examples shipped with SafeTerm include viewer, netops,
log-viewer, service-viewer, service-operator, config-viewer, config-operator
and the legacy config-approver role (no longer needed for approval).

    { "name": "viewer", "description": "Read-only diagnostics." }

Creating a role alone grants no operation. The identity must receive that role
and a policy must grant the desired operations. Configuration-management roles
also appear in the resource registry's read/propose authorization rules;
those permissions are separate from operation policies.

## 6. Policies: connect roles to operations

    {
      "role": "viewer",
      "operations": ["system_info", "memory_info", "disk_usage"]
    }

Every referenced role and operation must exist. Users can have multiple roles;
permissions are granted by matching operation policies. There is no implicit
root-like access merely because a role is named admin. Use the installed
catalog to inspect actual grants:

    sudo termctl role list
    sudo termctl role describe viewer
    sudo termctl role describe netops

Adding a role to an SSH user is an identity-management change. Adding operations
to that role is a signed catalog change. Keep both actions deliberate.

## 7. Profiles: where and with which privileges execution happens

A profile describes a separate Runner process and service identity:

- name: profile identifier referenced by operations and executable allowlists.
- executor_kind: process or systemd backend.
- socket_path: local IPC socket used to reach the Runner.
- run_as_user: dedicated operating-system identity.
- capabilities: approved Linux capabilities.
- memory_limit_mib and process_limit: declared resource bounds.
- controller_executable_ref and controller_args_prefix: typed controller fields
  for a systemd backend.

The shipped profiles are diagnostics, network-observer, log-reader and
service-manager. Network observation requires approved network capabilities;
read-only diagnostics do not inherit them. The service-manager uses a fixed
sudo/systemctl controller and a generated allowlist of exact privileged actions.

A profile is not a user role. A user can hold netops while an operation is
executed by the safeterm-network service account. Changing a JSON profile does
not automatically provision new system users, unit files or OS privileges;
its deployment assets and capability boundaries must agree.

## 8. Executables: approved binaries and integrity

    {
      "id": "uname",
      "path": "/usr/bin/uname",
      "sha256": "REPLACE_WITH_REAL_TARGET_SHA256",
      "allowed_profiles": ["diagnostics"]
    }

This example is illustrative: a real digest must contain exactly 64 hexadecimal
characters. id is referenced by executable_ref; allowed_profiles restricts
which Runner profiles may use it. A valid catalog signature does not replace
binary integrity checks. Runner verifies the exact executable, including its
canonical path and symlink behavior, and pins execution to an open descriptor.

Digests depend on the target distribution and package versions. OS updates to
ping, sudo or another registered binary can cause integrity mismatches. Prepare
and approve a new catalog for the updated binaries. Do not disable integrity
checks or blindly substitute hashes to silence an unexpected mismatch.

## 9. Signing, trust and local signing

catalog.sig is a detached Ed25519 signature over the normalized catalog model:
version, operations, roles, policies, profiles and executables. Ordering of
semantic collections is normalized; argv token order remains significant.
The trust public key verifies the signature. It is not an SSH login key.

The private signing key authorizes catalog changes and must be protected.
With --local-signing, the package installer prepares hashes on the target and
signs locally using /root/.config/safeterm/catalog-ed25519.key by default.
This simplifies evaluation but keeps signing authority on the runtime host.

With offline signing, prepare on the exact target, transfer the unsigned
catalog to the protected signing machine, sign there, then return the signed
catalog. Keep the private key off runtime hosts. Distinct OS/binary versions
may need distinct prepared catalogs across a fleet.

These README files and .jsonc templates are documentation, not signed catalog
fields. Package/bundle manifests cover their transferred bytes. They may be
updated without re-signing the catalog itself; modifying signed fields requires
a new signature.

## 10. Inspecting and using the installed configuration

From an ordinary administrator shell:

    termctl help
    sudo termctl catalog list
    sudo termctl catalog describe ping
    sudo termctl role list
    sudo termctl role describe netops
    sudo termctl config list
    sudo termctl config describe RESOURCE_ID
    sudo termctl user list
    sudo /usr/local/bin/safeterm-preflight --installed

Inspect another staged tree with the prefix:

    termctl --config-dir /path/to/catalog catalog list

Inside the authenticated SafeTerm restricted terminal:

    whoami
    list
    describe ping
    execute system_info
    execute ping host=127.0.0.1 count=5
    config list

The terminal uses execute, not run. Its list and completion respect the user's
permissions. termctl is the local administrator utility and uses OS permissions;
its inspection output is not an authentication or authorization decision.

## 11. Authoring and deploying changes

1. Work on a copy of the source catalog, not the live signed configuration.
2. Use TEMPLATE.jsonc files as references. Put active definitions in .json arrays.
3. Define or reuse an executable and the narrowest compatible Runner profile.
4. Define the operation with bounded parameters and fixed argv tokens.
5. Add the policy grant and assign the role to intended identities.
6. Prepare hashes on the exact deployment host:

    termctl catalog prepare --source /path/to/source --output /tmp/catalog-new

The alias termctl catalog regenerate-hashes accepts the same options. The
output directory must not already exist. Preparation writes normalized unsigned
catalog JSON; it does not copy identity maps, signatures or private signing keys.

On a package-only host, the corresponding helper is available in the extracted
package as ./payload/bin/safeterm-catalog-prepare. It is not a shell command
executed through the restricted SafeTerm terminal.

Sign on the protected signing machine, using the package-provided tool:

    ./payload/bin/safeterm-catalog-sign \
      -config /tmp/catalog-new \
      -private-key /secure/catalog.key \
      -signature /tmp/catalog-new/catalog.sig

For initial key generation, use -generate-key with -private-key and -public-key;
place the public key at /tmp/catalog-new/trust/catalog-ed25519.pub. Reuse the
approved key for subsequent catalogs unless deliberately rotating trust.

Install using the verified extracted package:

    sudo ./install.sh --signed-catalog /path/to/catalog-new --dry-run
    sudo ./install.sh --signed-catalog /path/to/catalog-new

Fresh installations also need the SSH enrollment options shown by install.sh
--help. Existing identities and managed values are preserved on upgrades.
Preparation and signing alone do not activate a deployment. Use the installer
and preflight instead of partially replacing live files or restarting only one
service after a cross-cutting catalog change.

## 12. Common problems and boundaries

Unknown operation: inspect the installed catalog and the user's authorized menu.
Permission denied: inspect policies and the server-owned identity role mapping.
Bad parameters: use describe and respect required values, aliases and ranges.
Signature failure: verify the approved key and re-sign intended catalog changes.
Digest mismatch: inspect the target binary/package update and prepare anew.
Missing executable: install the required OS package before catalog preparation.
Missing role in config workflow: inspect config-manager/resources.json separately.

JSON is strict: comments belong in .jsonc examples, not active .json files.
Back up authorship sources outside loaded directories. Keep /etc/safeterm
root-owned and retain the installer-managed permissions on identity/private-key
files. Do not broaden Runner privileges to bypass a validation error.

Related source documentation: docs/catalog-authoring.md, docs/termctl.md,
docs/ssh-authentication.md, docs/configuration-management.md and
docs/offline-installation.md. These paths are in the source repository; this
standalone guide is the copy installed alongside the runtime configuration.

## 13. Single-operator configuration changes

Config edits always use the same identity to propose and apply after preview
and y/N confirmation. No second user is required. propose_roles controls both
steps. Legacy apply_roles is normalized to propose_roles; legacy
require_separate_approval is ignored. A config-approver-only identity cannot
apply other users' proposals. Read, activation and rollback permissions remain
separate. The --single-operator installer flag is now a compatibility no-op.

## 14. Contextual terminal help

Alongside Tab completion, type ? and press Enter for main commands with short
descriptions. Use execute ? for authorized operations, execute ping ? for
parameter help, config ? for config subcommands, and config edit ? for
authorized resources. Separate ? from preceding words with a space. Help
never executes an operation or edits a file and returns to the prompt.

## 15. Internal Console authentication

Console users no longer map one-to-one to Linux accounts. One restricted Linux
entry account (`safeterm`) launches the Console, which prompts for an internal
username and password before exposing the menu. Provision each operator with:

```sh
sudo termctl user add console --id alice --roles viewer,netops,config-viewer
sudo termctl user password console --id alice
sudo termctl user auth console --id alice --roles viewer,config-viewer
sudo termctl user list
sudo -iu safeterm
```

Passwords are entered twice without echo and stored only as bcrypt hashes in
`/etc/safeterm/console/users.json`, owned by root:safeterm-identity mode 0640.
Only the isolated Identity daemon can read these hashes; the shared account
cannot. Internal roles are verified server-side using an opaque session bound
to the Console process UID/PID. Sessions expire after 30 minutes by default.
Changing a password or role invalidates the old session; no restart is needed.
SSH users remain a separate identity list with independent credentials.

Legacy `os_user` entries are accepted for migration only. Without an internal
hash they are disabled, so upgrade requires provisioning their internal password
once with `sudo termctl user password console --id EXISTING_ID`. `passwd safeterm`
changes only the separate Linux entry password, not the internal user password.
The installer does not change PAM or create a Linux account for each operator.
See the repository's `docs/console-authentication.md` and ADR-025 for details.

## 16. Apply catalog changes on an installed host

After editing approved command definitions or log aliases, run from the ordinary
administrator shell:

    sudo termctl catalog apply --dry-run
    sudo termctl catalog apply

Apply reuses the existing local signing key, validates the catalog, checks pinned
executable hashes and user/resource roles, regenerates sudoers, and restarts only
previously running catalog services. It does not replace users, passwords, managed
application values or the trust anchor. Active sessions must reconnect.

Use --source DIR to apply a reviewed copy, --key-file FILE for an existing custom
local key, or --signed-catalog DIR for a catalog signed with the approved offline
key. No new key is generated and executable hashes are not refreshed implicitly.
The earlier prepare/sign instructions remain useful for offline signing.

    sudo termctl catalog rollback

This restores the previous catalog or recovers an interrupted catalog transaction;
it differs from package-level termctl rollback. Installation seeds a verified
checkpoint and retains one previous catalog. A pending recovery blocks new apply,
update, rollback and uninstall until catalog recovery succeeds. Missing historical
content cannot be reconstructed: if both live signature and checkpoint are invalid,
restore a valid catalog before applying. Log file ACLs and logrotate remain site
configuration. See repository docs/catalog-activation.md and ADR-026 for details.


## Preserve and transfer customized catalogs

Package updates retain the installed verified catalog, including custom operations
and log aliases. New defaults are stored separately at /usr/share/safeterm/defaults
and never automatically merged into active permissions. Existing user mappings,
SSH host keys, Config Manager registry, unrelated site files and managed application
values are retained. Managed files are seeded only when absent; existing contents,
ownership, permissions and ACLs are not replaced by package examples.

Export portable catalog definitions on your configured server:

    sudo termctl catalog export --output /root/site-catalog

Copy that directory to another compatible, locally signed installation and run:

    sudo termctl catalog apply --source /root/site-catalog --refresh-hashes --dry-run
    sudo termctl catalog apply --source /root/site-catalog --refresh-hashes

Export excludes identities, passwords, SSH keys, signing keys, signatures and
Config Manager resource definitions. The target uses its own signing key/trust.
The explicit refresh approves executable content on that target; ordinary updates
preserve approved hashes. Resource registries and log paths/permissions must be
provisioned separately. Do not clone all of /etc/safeterm across machines.

Finish intended edits with termctl catalog apply before an update. Unapplied
changes, incompatible profiles and executable digest mismatches stop validation;
updates do not reset site definitions to make checks pass. Catalog rollback restores
the previous activation; package rollback restores the pre-update snapshot.
