# Ubuntu Server 24.04 network configuration

SafeTerm reads Netplan YAML, presents typed interface/route commands and keeps a private candidate until the user explicitly applies it. This feature targets Ubuntu Server 24.04 with the `networkd` renderer and `netplan.io`. Installing or updating SafeTerm never applies network configuration.

## Administrator setup

The root-owned policy is `/etc/safeterm/network/policy.json`. An empty `interfaces` list (the installed default) allows no network edits. Set exact interface names after inspecting `ip link` and the existing Netplan files:

```json
{
  "version": 1,
  "interfaces": ["ens160", "ens192"],
  "allow_all_interfaces": false,
  "files": [],
  "new_interface_file": "",
  "read_roles": ["netops", "network-viewer", "network-operator"],
  "edit_roles": ["netops", "network-operator"],
  "confirm_seconds": 120,
  "allow_generated_files": false
}
```

`files` optionally restricts writable sources to exact `.yaml` basenames. An empty list discovers `/etc/netplan` filenames automatically; it does not assume `00-installer-config.yaml`, `50-cloud-init.yaml`, or any other filename. The daemon reads the policy on each request; no catalog signing or daemon restart is necessary for a policy edit. Keep it root-owned and without group/world write access. Grant the relevant SafeTerm identity the `network-viewer` or `network-operator` role using `termctl user` in addition to its existing roles. The `netops` role has network read/edit privileges by default, restricted to the interfaces/files explicitly permitted by policy.

A cloud-init marker makes an interface read-only by default. Setting `allow_generated_files` acknowledges that the administrator has arranged for cloud-init or another provisioning system not to overwrite that configuration on the next boot. SafeTerm does not disable cloud-init itself.

## Current and future hardware

Set `"allow_all_interfaces": true` to allow the configured edit roles to manage
all present and future supported Ethernet NICs, including VM NICs and newly
attached LAN modules. The default remains false. This setting permits current
interfaces outside the exact `interfaces` list as well; it is not limited to
hardware added after installation. Role, renderer, source-file, generated-file
and unsupported-configuration restrictions still apply.

The interface table and detail view display the kernel MAC address so an admin
can compare it with the VM/hypervisor hardware settings. Newly discovered NICs
appear even when they do not yet have a Netplan entry. Their definitions are added
to an existing eligible file in `/etc/netplan`; SafeTerm does not create a separate
Netplan file. The save/apply/confirm flow is unchanged. Existing sources remain
untouched until confirmation, and rollback restores their original bytes.

If only one eligible administrator-owned source exists, SafeTerm selects it.
If several exist, choose an existing basename in `/etc/safeterm/network/policy.json`:

```json
"new_interface_file": "01-netcfg.yaml"
```

Replace `01-netcfg.yaml` with your actual existing filename. An empty value uses
automatic selection. The selected file must also pass `files`, renderer,
cloud-init ownership and source-trust checks. Missing or ambiguous destinations
are read-only with an explanation; terminal users cannot supply arbitrary paths.
A legacy `90-safeterm-interfaces.yaml` is excluded from automatic selection when
another eligible source exists. Existing entries keep their original source,
including legacy files; sources are not silently moved or deleted.

Inline YAML mappings such as `ethernets: { ... }` are supported. When a changed
source needs it, SafeTerm converts that file to ordinary indented block YAML in
the candidate, retaining comments and unknown properties. Confirmation commits
the formatted source; rollback restores the exact original bytes. Newly inserted
interface definitions use block style so they remain editable after reopening.

## Persistent interface names

Inspect and change a name inside the interface editor:

```text
config network interface
edit ens192
show
set name lan0
diff
save
execute network apply
execute network confirm
```

`set name` stages a persistent name alongside addresses, DHCP, DNS and MTU.
The dirty prompt, `show` and `diff` display it. `exit` discards the selected
item's unsaved edits; `next` keeps them staged; `cancel` discards the editor.
`save` validates and stores the private candidate without changing Netplan or
`.link` files. The existing timed apply/confirm flow commits Netplan and the
naming rule together. Trial timeout or rollback leaves the original naming
rule in place. A confirmed rollback restores both sources while the hardware
names remain unchanged; after a reboot/name activation, administrator recovery
is required before restoring an old naming rule.

The requested name takes effect only on the next reboot/device activation.
The live trial does not install a `.link` override or rename the active NIC.
Names must start with a letter, contain letters/digits/underscore/hyphen, and
be at most 15 characters. Current and requested names must be allowed by policy;
`allow_all_interfaces` covers both. Conflicting live/persistent names and
multiple interfaces requesting the same name are rejected. The old
`execute network rename` command now directs operators to this editor flow.

SafeTerm searches `/etc/systemd/network/*.link` for an existing single-MAC rule.
For example, it updates the `Name=` in your existing `10-persistent-net.link`:

```ini
[Match]
MACAddress=00:0c:29:ef:e4:51

[Link]
Name=lan0
```

It preserves comments, unrelated settings, permissions, ownership and supported
extended attributes. `NamePolicy=` is cleared so a configured naming policy does
not override the requested name. Shared/multiple MAC matches, multiple matching
files, advanced matches, symlinks, writable/untrusted files and drop-ins are
rejected for administrator resolution rather than guessing.

If no matching rule exists, SafeTerm uses the first available numbered filename,
starting at `/etc/systemd/network/11-persistent-net.link`, then `12-`, `13-` and
onward through `98-persistent-net.link`. It leaves `10-` for administrator rules.
Occupied filenames and matching drop-in directories in `/etc`, `/run`,
`/usr/lib` and `/usr/local/lib/systemd/network` are skipped. Existing matching
files keep their names, including MAC-based files from v0.34.0. It also binds the
Netplan definition to that MAC and removes conflicting `set-name` entries, so
its IP settings continue matching after the interface changes name. Existing
Netplan IDs and unrelated configuration remain intact. The detail view shows
the persistent name and whether it differs from the current live name.

The candidate is authorized and audited by the dedicated network manager,
uses the lifecycle lock, validates Netplan in a temporary root, and journals
persistent writes for crash recovery. Fully written naming commits are finalized on startup instead of undoing a name
already consumed during reboot. Incomplete trials restore the old sources;
changed hardware identities or external edits block automatic restoration and
require a Linux administrator.
Review local earlier-priority systemd rules and any initramfs copies when
planning a reboot. See [systemd.link](https://www.freedesktop.org/software/systemd/man/systemd.link.html).

## Read, edit and save a candidate

`?` followed by Enter shows contextual help. Tab retains the existing Linux-style completion behavior. Help lists one choice per line with a short English description. For example:

```text
?                       Editor commands and their effects
set ?                   Editable fields, formats and limits
set dhcp4 ?             Meaning of enable and disable
set addresses ?         CIDR address syntax
unselect ?              Lists supporting removal of selected values
edit ?                  Existing items, including read-only reasons
save ?                   Save semantics, without saving anything
execute network ?       Status, candidate and timed trial actions
execute network apply ? Trial semantics, without applying anything
```

Help inside an editor uses its cached view; it never selects an item, changes
staged values, saves a candidate or contacts the privileged manager. Tab continues
to insert/complete tokens and list alternatives using the existing Linux workflow.


```text
config network interface
    show
    edit ens160
        set dhcp4 disable
        set addresses 192.168.10.20/24
        append addresses 192.168.10.21/24
        set dns 192.168.10.2 1.1.1.1
        set search-domains example.internal
        set mtu 1500
        show
        diff
    next
save
```

`set addresses` replaces the static address list. `append addresses` adds secondary addresses. `unselect addresses 192.168.10.21/24` removes a selected address. `unset addresses`, `unset dns`, `unset search-domains` clear the respective list; `unset mtu` removes the explicit MTU. DHCP is `set dhcp4 enable|disable`. Turning DHCP on does not silently remove static addresses. MTU is limited to 576–65535; the physical device/backend may impose tighter limits, reported by Netplan.

In the interface context, `show` displays a compact table of names, kernel link states, DHCP4, addresses and editing access. `edit NAME` followed by `show` displays that interface's DNS, search domains, MTU, source files and readable restriction details. A `*` marks an interface with unsaved edits. Editing access describes policy. STATE is the kernel operational state recorded when the editor opens, or unknown when unavailable. Address and DHCP columns show configuration values. Use `execute network status` for fresh live details. `diff` displays field changes using removed (-) and added (+) lines. `next` leaves the selected item; `save` validates and saves the candidate, without changing `/etc/netplan` or the live network. `cancel` discards only unsaved editor changes. `execute network discard` discards the saved candidate. Candidates are private to one SafeTerm identity and survive reconnects. Another operator cannot overwrite that candidate; its owner must discard it first.

## Routes and default gateway

At config network route, show lists routes by a short ID with destination,
gateway, interface and metric columns. IDs remain stable while the editor is
open, including after deleting or adding routes. They are session selectors;
reopen show after reconnecting. Generated route hashes stay internal to the
presentation. Select and delete routes using only the displayed IDs; stored
names are internal and are not accepted as CLI selectors.

```text
config network route
show
edit 1
show
set gateway 192.168.222.1
diff
next
save
```

A selected route's show uses labeled Destination, Gateway, Interface and Metric
values. 0.0.0.0/0 is the IPv4 default route; ::/0 is the IPv6 default route.
Gateway is an IP address without a CIDR prefix. No gateway means a direct route.

Create a route with add. SafeTerm assigns the next available editor ID and
opens that route. Use set commands to fill in its fields:

```text
config network route
add
set interface ip0
set destination 10.50.0.0/16
set gateway 192.168.222.1
set metric 100
diff
next
save
```

Use edit ID to select an existing route; an unknown edit is an error and cannot
accidentally create a new route. add takes no name or ID and never saves a route.
The new route needs an interface and network destination before it can be saved.
Gateway and metric are optional: omitting them means direct routing and an
automatic metric. show displays Not set for missing required fields.

Use delete ID to stage removal of that exact route. IPv4 and IPv6 default routes
(0.0.0.0/0 and ::/0) cannot be deleted. The table's DELETION column and route details
identify this protection; delete ? and delete followed by Tab offer only routes
that can be deleted. Protection is enforced by the network manager at save and
apply, including candidates from older releases. Existing default routes can
still be edited, for example with set gateway. set ? describes the fields and
set interface followed by Tab lists editable interfaces.

exit from a route discards its edits and returns to the route list. next keeps
its edits staged. Neither applies networking; the save/apply/confirm flow below
remains required. Underlying route identities, ownership and policy checks are
unchanged by the short display IDs.

## Timed apply, confirmation and rollback

```text
execute network status
execute network diff
execute network apply
execute network confirm
```

`apply` first runs `netplan generate --root-dir` against a private copy of all inputs. It then creates temporary, same-basename `/run/netplan` overrides and starts `netplan try`. Persistent `/etc/netplan` files remain unchanged during the trial. The response gives the confirmation deadline, normally 120 seconds. Reconnect if necessary and confirm as the same SafeTerm identity and broker type. No second person's approval is involved.

`confirm` checks for concurrent source/policy edits, writes the original files atomically while preserving ownership, mode and extended attributes, and confirms Netplan. Only then are the temporary overrides removed. A timeout, failed trial, daemon shutdown, or daemon restart with an unfinished journal triggers rollback and an independent `netplan apply` of persistent configuration. This reduces remote lockout risk, but cannot guarantee connectivity if the OS/backend fails to restore networking; retain out-of-band access when changing a management interface.

```text
execute network rollback
```

During a trial this rejects the trial. After confirmation it restores the last confirmed change, provided the relevant files have not since been edited externally and current policy still permits its targets. One last confirmed backup is kept; history does not grow indefinitely. Updates and package rollback refuse to proceed while a trial is pending.

## File discovery and preservation

Netplan combines `/lib/netplan`, `/etc/netplan` and `/run/netplan`. Same-basename files in `/run` shadow `/etc`, which shadows `/lib`; remaining filenames merge lexicographically. SafeTerm tracks that provenance, including settings split over several files. It edits only administrator-allowed `/etc/netplan` sources. An interface partly defined outside `/etc/netplan` remains read-only. A concurrent source or policy change invalidates an older candidate; use `execute network discard`, then start again.

Unrelated bytes remain unchanged. Within an edited field block, YAML formatting can be normalized and comments can move, but comment text is retained. List fields contributed by several files are consolidated to prevent Netplan concatenating removed addresses back into the result. Ownership, permission bits and xattrs/ACLs are preserved; root-owned files do not need to be chowned to a service account. Symbolic-link files, hard links, writable/untrusted sources, duplicate YAML keys, anchors and aliases are refused. Input files and structure have bounded sizes.

Only existing Ethernet definitions with an unambiguous present device, including simple MAC/name matches, are editable. Virtual device creation, bridges, bonds, VLANs, Wi-Fi, NetworkManager, policy routing, advanced route attributes and legacy `gateway4`/`gateway6` conversion are outside this version. Advanced route entries are retained read-only. Migrate legacy gateways to `routes` as an administrator before editing that interface. A static address requires CIDR; DNS requires IP literals. IPv6 address/route values are accepted, but DHCP6/router-advertisement policy is not edited.

## Process boundaries and testing

The SSH and Console brokers remain unprivileged. A separate root service, `safeterm-network-manager`, accepts bounded typed requests over a Unix socket, checks peer credentials, validates Console sessions or reloads SSH roles, and enforces policy again. It accepts no shell, arbitrary command, YAML payload or caller-supplied file path. Edit authorizations and outcomes enter the existing audit store. Network state/candidates/journals live in `/var/lib/safeterm/network` with root-only access.

Manage the daemon with `sudo termctl service status network-manager`. Its journal is available through `sudo journalctl -u safeterm-network-manager`. The policy and state survive ordinary updates; SafeTerm uninstall does not remove your Netplan files.

Unit/CLI tests cover discovery, merging, validation, source conflicts, permissions, metadata, candidate ownership and recovery. `tests/netplan/disposable.sh` runs real Netplan trials in an Ubuntu 24.04 systemd-nspawn guest with a separate network namespace, including confirmation, rollback and timeout. Release packaging depends on this CI job as well as the existing systemd installation test.

References: [Netplan file merging](https://netplan.readthedocs.io/en/stable/netplan-generate/), [Netplan try](https://netplan.readthedocs.io/en/stable/netplan-try/).

## Navigation and help

Login prints the main command menu once. Entering, selecting or leaving a context
is quiet. Use ? (or COMMAND ?) explicitly for help; help keeps the input prefix.
Tab in a configuration menu lists only its child names, such as interface and
route. At an editor item list it offers item names or route IDs. Inside a selected
item, use set followed by Tab for field names; empty Tab does not dump commands.

exit moves up one level and discards unsaved edits to the item being left, naming
it if changes were discarded. Other staged items remain. At an item list, exit
leaves that editor and discards its remaining draft. At the root prompt only,
exit closes the session. Ctrl+] closes the whole session from any level.
The former back command is removed.

next leaves a selected item while keeping edits staged; save validates and
commits the editor. Network save stores a candidate for the separate timed
execute network apply flow. cancel discards the current editor and returns to
root. Parent-menu exit discards retained logical drafts under that menu.
whoami is available only at root. clear remains available in all contexts.

Choose a menu child by typing its name, or use edit with a relative path. From
the config menu, edit network interface opens the interface list directly.
Navigation never saves a file, applies networking or prints an automatic menu.


## Draft display and session controls

A `*` after the prompt context means unsaved edits exist. Interface summaries
include STATE and show `Read-only (Policy restricted)` for policy denials.
Interface details use labeled values, `System default` for inherited DNS and
search domains, and `(staged)` for edited fields. Local diff uses `-` and `+`
lines. A bare IP is rejected immediately with Address/Prefix guidance.

Use `save` to validate and commit the candidate, `cancel` to discard the editor,
`exit` to move up while discarding edits to the item, and `next` to move up while
keeping edits staged. `exit` moves up one level; `whoami` is root-only and `clear` works everywhere.
Leaving a dirty item discards its edits. Saving a network candidate never applies
networking; the confirmed trial still starts with `execute network apply`.
