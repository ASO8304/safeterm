# ADR-054: Linux Console mappings and independent network drafts

Status: Accepted

Console operators may authenticate once through individual Linux accounts.
Root-managed `/etc/safeterm/access/console/linux-users.json` explicitly maps
Linux account names to SafeTerm user IDs and catalog roles. These mappings have
no password hashes. Existing shared-account internal login remains available
for upgrades. Old `os_user` fields in the legacy credential file still grant no
access; administrators opt into the new mapping file explicitly.

Identity obtains UID/PID from `SO_PEERCRED`, resolves enabled account mappings,
audits login, and issues expiring process-bound opaque sessions. Gateway, network
manager and configuration managers verify these sessions and replace claimed
roles with authoritative roles. Only configured UIDs reach Console APIs, and
Console operators cannot use the verifier API or claim SSH Broker credentials.
Role/mapping changes revoke sessions on their next verification. Adding account
UIDs requires restarting services. Linux account, password, shell and group
provisioning remain explicit administrator actions.

Saved network candidates belong to authenticated transport and user, with hashed
filenames under the private manager state directory. A different user's saved
candidate cannot lock editing. Existing global candidates migrate only for their
owner. One live trial remains global, owner-checked, timed and serialized.

Netplan definitions for absent devices are preserved on disk but excluded from
active interface/route editing. Hardware/source changes invalidate saved draft
bases; reopening shows current hardware and warns about the retained stale
candidate. Explicit replacement saves archive that candidate. Old editor versions
and stale apply attempts still fail closed; operators reopen before saving.
Live status and explicit discard work independently of malformed source/draft
data. These changes do not remove source conflict checks or timed rollback.

Netplan file owner/mode restrictions are intentionally removed at the user's
request. Parent directory, symlink, hard-link, regular-file, size and transaction
conflict checks remain. Policies, identity mappings, `.link` rules and private
state retain their ownership and permission requirements.

Terminal presentation is root-managed in `/etc/safeterm/terminal.json`. Fixed
placeholders and bounded printable text prevent escape or shell evaluation.
Context and dirty markers cannot be omitted from the prompt. Exit asks for
explicit confirmation before discarding affected drafts; empty input, refusal,
Ctrl+C or EOF retain the draft. Ctrl+] remains the emergency close mechanism.
