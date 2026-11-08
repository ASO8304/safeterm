# ADR-056: New Console accounts and independent installer enrollment

Status: Accepted. Supersedes ADR-028's shared initial SSH/Console enrollment and
ADR-054's manual OS-account provisioning for new users. Existing mappings remain
supported without converting or modifying their Linux accounts.

Root's `termctl user add console --os-user NAME --roles ...` performs complete
provisioning of a new local Linux Console account. `--id` defaults to NAME.
Existing users, same-name groups and existing home paths are refused. Root and
service-account names are excluded. There is no takeover/adoption flag. Invalid
roles, duplicate identities, unavailable names, failed password confirmation or
nonterminal input fail before account creation. Availability is rechecked after
password input and acquisition of the lifecycle lock to handle concurrent
administration. A custom `--config-dir` cannot trigger host account provisioning.

The fixed useradd invocation creates a private primary group, a home directory,
only the required SafeTerm socket groups and the installed SafeTerm login shell.
It grants no sudo or unrelated groups and never invokes usermod on an existing
account. New passwords use the existing confirmed, non-echoing terminal reader
and complexity policy, stay in bounded transient memory, and reach the fixed
system chpasswd/PAM tool only over stdin. They never enter command arguments,
environment variables, JSON or temporary files. Existing SSH/internal Console
credentials are not read or changed by this flow.

The account starts password-locked. The command publishes its mapping, restarts
SafeTerm consumers and waits for sockets/service health before initializing the
Linux password. A live network trial or other lifecycle action prevents this
operation. Existing SafeTerm sessions must reconnect after the restart. Failure
removes only the new mapping and locks only the newly created UID/name; it never
adopts another account or deletes home data. Account-creation failures can leave
a new locked OS account for administrator review. Mapping/role editing and
deletion remain separate operations; deleting a mapping never deletes a Linux
account or its files. Site SSH/PAM/sudo policies remain administrator-controlled.

The fresh installer first enrolls the SSH service identity with its own username
and password/key. Once installation is committed and releases its lifecycle
lock, a separate optional Console step delegates to the same termctl provisioning
command. Its username/password are independent; existing names are rejected and
the operator can choose another. SSH hashes are never copied into a Console
store. Upgrades preserve existing identities and do not repeat enrollment.
Noninteractive and no-start installations defer Console creation to termctl.
Installer package rollback still restores account database snapshots; ordinary
maintenance access and review of account/home state remain necessary when using
package rollback after subsequent account changes.

Unit tests use fake OS operations and temporary mappings, covering refusal,
races, cancellation and rollback. Installer orchestration tests isolate both
enrollment steps. Real creation, shell/groups, password setup, independent stores,
existing-account preservation and subsequent single-login Console/password use
are exercised only on disposable installation CI, never on the developer host.
