# Console authentication

## Single login through an individual Linux account

Console operators can use their existing Linux login, mapped explicitly to a
SafeTerm identity and roles. After Linux authenticates the account, the Console
opens without another Username/Password prompt. SSH authentication is unchanged.

Provision an existing individual account from a normal Linux administrator shell:

```sh
sudo usermod -a -G safeterm-clients,safeterm-gateway alice
sudo termctl user add console --id alice --os-user alice --roles viewer,netops,config-viewer,config-operator
sudo termctl service restart all
```

Use only roles appropriate for that operator. Group membership takes effect on
a new Linux login. From that login, run `termctl` without sudo. For a restricted
account that should enter SafeTerm immediately at Linux login, set its shell:

```sh
sudo usermod --shell /usr/local/bin/safeterm-console alice
```

SafeTerm does not create the account, set its Linux password, change its shell,
or grant groups automatically. Keep a separate Linux maintenance account. The
mapping is stored without secrets in
`/etc/safeterm/access/console/linux-users.json`, root-owned mode 0644:

```json
{
  "users": [
    {
      "os_user": "alice",
      "user_id": "alice",
      "roles": ["viewer", "netops", "config-viewer", "config-operator"]
    }
  ]
}
```

The file is optional and defaults to no mappings. Root, the shared `safeterm`
account and SafeTerm service accounts cannot be mapped. Duplicate account names,
SafeTerm IDs and Linux UIDs are rejected. Old `os_user` fields in legacy
`users.json` remain migration metadata and do not authenticate anyone.

Identity obtains the connecting process's UID/PID from the kernel, rather than
trusting environment variables or caller-supplied names. It issues the same
audited, expiring, process-bound tokens used by legacy Console authentication.
Every Gateway/network/configuration boundary verifies the token and supplies
authoritative roles. Linux operators cannot use the privileged token-verifier
API or impersonate the SSH Broker. Removal, disabling or role changes invalidate
sessions on their next verification; streams recheck within 15 seconds. A new
Linux UID requires a coordinated service restart. Linux account names and UIDs
should not be recycled while their SafeTerm mappings remain enabled.

### Linux account restrictions are a separate boundary

The mapping grants SafeTerm roles; it does not remove existing Linux authority.
For an operator who must be restricted to SafeTerm, set the fixed Console login
shell, remove sudo grants and unrelated privileged groups (including equivalents
such as Docker/LXD access), and restrict other login/file-transfer entry points.
An account that can already obtain root remains a Linux administrator regardless
of its SafeTerm roles. Review sudoers, groups, SSH configuration and writable
privileged files when provisioning the account. SafeTerm does not perform these
changes automatically.

Linux/PAM authenticates the account. `/etc/passwd` contains the account UID, home
and shell; local password hashes normally live in `/etc/shadow`, not the SafeTerm
mapping. Identity uses kernel UID/PID credentials and root-controlled role
mappings, rather than trusting `USER`, environment variables or the prompt name.
The Console runs without root, disables dumpability and enables no_new_privs.
Execution/configuration services independently enforce authorized operations,
parameter constraints, file boundaries and audit requirements. These controls
reduce escalation paths; they are not an absolute guarantee against kernel,
PAM or SafeTerm vulnerabilities, or against a compromised Linux administrator.

Update and remove mappings with:

```sh
sudo termctl user auth console --id alice --os-user alice --roles viewer,config-viewer
sudo termctl user del console --id alice --os-user alice
sudo termctl user list
```

At the root SafeTerm prompt, `passwd` (alias `change_pass`) verifies the current
password and asks twice for the replacement without echo or history. For a mapped
Console account it changes that account's Linux password through the system
passwd/PAM policy. For a legacy internal Console account it updates only
`access/console/users.json`; for SSH it updates only `access/ssh/users.json`.
It cannot select another user, change roles or grant sudo. Linux administrators
can still use `sudo passwd alice` outside SafeTerm. Package upgrades preserve
both mapping and credential files. See [terminal presentation](../configs/docs/terminal.md)
for branding, prompt formatting and confirmation before discarding drafts.

## Legacy shared-account login

Console operators have SafeTerm usernames and passwords, independent of Linux
accounts and SSH users. One shared, unprivileged Linux account, `safeterm`, starts
the fixed `/usr/local/bin/safeterm-console` login shell. That shell requires an
internal login before showing the restricted terminal. It never opens Bash.
There is no per-person `useradd`, and the installer does not modify Linux PAM.

## Request path

```mermaid
sequenceDiagram
    actor Operator
    participant Console as Console (shared Linux UID)
    participant Identity as Isolated Identity service
    participant Gateway
    participant Manager as Config Manager
    Operator->>Console: Internal username / password
    Console->>Identity: Login over authenticated Unix socket
    Identity->>Identity: Verify bcrypt + rate limits
    Identity-->>Console: Random session, roles, expiry
    Console->>Gateway: Operation + session
    Gateway->>Identity: Verify session against kernel peer UID/PID
    Identity-->>Gateway: Authoritative user and roles
    Gateway->>Gateway: Authorize catalog operation
    Console->>Manager: Config request + session
    Manager->>Identity: Verify session against kernel peer UID/PID
    Identity-->>Manager: Authoritative user and roles
    Manager->>Manager: Authorize resource and validate fields
```

Gateway routes accepted operations to the existing isolated Runners. Identity
never executes commands or edits application configuration. The existing SSH
Broker keeps its own authentication and `/etc/safeterm/access/ssh/users.json`; creating
an SSH user does not create a Console user, even when their names are equal.

## Provisioning and login

After installing or updating the complete package:

```sh
sudo termctl service status identity
sudo termctl user add console --id alice --roles viewer,netops,config-viewer
# New password: (hidden)
# Confirm password: (hidden)
sudo -iu safeterm
# Username: alice
# Password: (hidden)
# safeterm::alice>
```

No Linux account named `alice` is required. The entry account is initially
password-locked; an authorized Linux administrator can use `sudo -iu safeterm`.
If the site permits direct physical Linux login, set the separate shared entry
password with `sudo passwd safeterm`, then log in as `safeterm` and complete the
mandatory internal login. SafeTerm does not enable Linux autologin. Sharing the
entry credential does not share internal identities, roles or audit attribution.

```sh
sudo termctl user list
sudo termctl user auth console --id alice --roles viewer,config-viewer
sudo termctl user password console --id alice
sudo termctl user del console --id alice
```

The last configured identity cannot be deleted with `termctl`. Fresh packages
include disabled `local-admin` as a placeholder; no default internal password
exists. Enable it by setting a password, or create your own user and delete the
placeholder. For unattended provisioning, use `--password-hash-file FILE` with a
bcrypt hash, never a plaintext password argument. Protect that input file.

Interactive password entry enforces 8–72 printable ASCII characters, uppercase,
lowercase, and a number. Bcrypt uses cost 12 when generated by SafeTerm. Existing
hash imports accept the same bounded bcrypt costs as SSH. A hash cannot prove
that its original plaintext satisfied complexity; provisioning must enforce it.
There is no Console public-key login: optional public keys belong to SSH.

User changes take effect without restarting services. Changing roles, password,
disabled state or deleting the user invalidates old sessions at their next
verification. An already-running stream is rechecked every 15 seconds; the
absolute authentication expiry also cancels execution. Already accepted bounded
unary/config transactions finish under their existing operation deadline;
revocation cannot undo a committed edit. Login again after changing credentials.

## Historical upgrade from pre-internal-login OS-user mappings

Upgrade through `sudo termctl update PACKAGE_DIR` using your normal local or
offline signing mode. The package preserves existing Console records and sets
the new credential ownership. Legacy `os_user` fields are migration metadata only;
they never authenticate a user. Entries without an internal bcrypt hash are
disabled rather than silently retaining Linux-UID-based access.

For the typical legacy record with `user_id: safeterm`:

```sh
sudo termctl user password console --id safeterm
sudo -iu safeterm
# Username: safeterm
# Password: the internal password just set
```

For other legacy records, use their existing `user_id`. Setting the internal
password enables that record and removes its old credential-file OS mapping.
A roles-only update does not enable a disabled record that already has a hash.
The upgrade does not delete existing Linux accounts or change their passwords.
Per-person Linux login is now supported through the separate `linux-users.json`
mapping described above; `termctl user add console --os-user` explicitly provisions
that mapping. Old credential-file `os_user` metadata does not grant access.
Review/removal of unrelated Linux accounts is an administrator decision.
Keep an ordinary administrator session available
while upgrading; old Console processes lose access to the new authenticated
boundaries. SSH remains available according to its independent configuration.

## Credential and session boundaries

- `/etc/safeterm/access/console/users.json` is root:safeterm-identity mode 0640. Its
  parent directories must be root-owned and not group/world-writable. The
  shared account must never join the `safeterm-identity` group. Identity refuses
  unsafe credential ownership, extended credential ACLs or entry-account group membership at startup; reads reject symbolic/hard links,
  excess size and insecure permissions.
- `safeterm-identity.service` runs as a separate non-root, nologin service user,
  with a read-only filesystem, no capabilities, Unix sockets only, no core dumps
  and finite memory/task limits. Service accounts are infrastructure accounts;
  their count does not grow with the number of Console operators.
- Identity accepts internal login from the shared Console UID and Linux login
  from explicitly mapped UIDs; logout is bound to the session's UID/PID. Gateway,
  the three Config Managers and trusted root helpers can verify sessions.
  Operators cannot use that verifier API. Every client verifies the daemon UID;
  every server obtains credentials with `SO_PEERCRED`.
- Each login issues a cryptographically random 256-bit token, stored by digest
  in daemon memory, bound to kernel UID/PID and an absolute expiry. Possession of
  the shared UID or a caller-supplied role is insufficient. Gateway and Config
  Managers replace caller roles with the authority's roles. Unary, streaming,
  termination and configuration paths all enforce this boundary. Config proposal
  tokens are also bound to the originating Console session.
- Default lifetime is 30 minutes, configurable from one minute to twelve hours
  via the Identity unit's `--session-ttl` flag. There is no persistent token or
  refresh credential. Logout removes the session; restarting Identity invalidates
  all sessions. Run `sudo termctl service restart all` for coordinated recovery.
- Login allows at most 5 attempts per username per minute and 30 globally per
  minute, four concurrent bcrypt checks, 32 IPC connections and 1,024 sessions.
  Excess requests fail closed. Three failed prompts close that Console process.
  Unknown users and bad passwords receive the same generic rejection.
- Passwords and tokens are absent from terminal output/history and audit records.
  Console disables dumpability and new privileges before reading credentials.
  Identity logs authentication success/failure/logout through the existing Audit
  service; audit failure denies a new login. Execution/config audit remains in
  `/var/lib/safeterm/audit.jsonl`. Root can still inspect credentials and control
  services; protection against a compromised administrator/kernel is outside this
  boundary. Shared-UID processes can cause denial of service, so do not give the
  entry account a general OS shell or allow untrusted programs under that UID.

## Validation

The automated suite covers internal login, hidden credentials/no login history,
legacy denial, incorrect passwords, throttling, audit failure, expiry/logout,
role revocation, forged identities/roles, process-bound replay protection,
privileged verification-only IPC, stream authentication and operation cancellation
at expiry. Existing SSH/public-boundary tests must remain green.

On a disposable installed host, create two Console users with different roles,
open two `sudo -iu safeterm` sessions, and verify the login identity, `execute ?`, `config ?`
and differing permissions. Enter an incorrect password, then reset one user's
password from the administrator shell: its old session must fail subsequent
requests, while the other session continues. Check that the shared account
cannot read the credential file and that no per-person account was created:

```sh
sudo -u safeterm test ! -r /etc/safeterm/access/console/users.json
getent passwd alice   # no output unless an unrelated account already existed
sudo journalctl -u safeterm-identity --no-pager -n 25
```

For streaming revocation, observe the documented 15-second recheck interval.
Use Ctrl+C to terminate an operation and Ctrl+] to exit the entire restricted
session. Do not publish the credential file or raw session tokens when reporting
problems.
