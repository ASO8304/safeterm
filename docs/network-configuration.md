# Ubuntu Server 24.04 network configuration

SafeTerm reads Netplan YAML, presents typed interface/route commands and keeps a private candidate until the user explicitly applies it. This feature targets Ubuntu Server 24.04 with the `networkd` renderer and `netplan.io`. Installing or updating SafeTerm never applies network configuration.

## Administrator setup

The root-owned policy is `/etc/safeterm/network/policy.json`. An empty `interfaces` list (the installed default) allows no network edits. Set exact interface names after inspecting `ip link` and the existing Netplan files:

```json
{
  "version": 1,
  "interfaces": ["ens160", "ens192"],
  "files": [],
  "read_roles": ["netops", "network-viewer", "network-operator"],
  "edit_roles": ["netops", "network-operator"],
  "confirm_seconds": 120,
  "allow_generated_files": false
}
```

`files` optionally restricts writable sources to exact `.yaml` basenames. An empty list discovers `/etc/netplan` filenames automatically; it does not assume `00-installer-config.yaml`, `50-cloud-init.yaml`, or any other filename. The daemon reads the policy on each request; no catalog signing or daemon restart is necessary for a policy edit. Keep it root-owned and without group/world write access. Grant the relevant SafeTerm identity the `network-viewer` or `network-operator` role using `termctl user` in addition to its existing roles. The `netops` role has network read/edit privileges by default, restricted to the interfaces/files explicitly permitted by policy.

A cloud-init marker makes an interface read-only by default. Setting `allow_generated_files` acknowledges that the administrator has arranged for cloud-init or another provisioning system not to overwrite that configuration on the next boot. SafeTerm does not disable cloud-init itself.

## Read, edit and save a candidate

`?` followed by Enter shows contextual help. Tab retains the existing Linux-style completion behavior. Help lists one choice per line with a short English description. For example:

```text
?                       Editor commands and their effects
set ?                   Editable fields, formats and limits
set dhcp4 ?             Meaning of enable and disable
set addresses ?         CIDR address syntax
unselect ?              Lists supporting removal of selected values
edit ?                  Existing items, including read-only reasons
end ?                   Save semantics, without saving anything
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
end
```

`set addresses` replaces the static address list. `append addresses` adds secondary addresses. `unselect addresses 192.168.10.21/24` removes a selected address. `unset addresses`, `unset dns`, `unset search-domains` clear the respective list; `unset mtu` removes the explicit MTU. DHCP is `set dhcp4 enable|disable`. Turning DHCP on does not silently remove static addresses. MTU is limited to 576–65535; the physical device/backend may impose tighter limits, reported by Netplan.

The editor's `show` displays its candidate and source filenames. `diff` displays staged edits. `next` leaves the selected item; `end` validates and saves the candidate, without changing `/etc/netplan` or the live network. `abort` discards only unsaved editor changes. `execute network discard` discards the saved candidate. Candidates are private to one SafeTerm identity and survive reconnects. Another operator cannot overwrite that candidate; its owner must discard it first.

## Routes and default gateway

A gateway is represented as a default route, avoiding deprecated `gateway4` settings:

```text
config network route
    edit default-uplink
        set interface ens160
        set destination 0.0.0.0/0
        set gateway 192.168.10.1
        set metric 100
    next
    edit office
        set interface ens160
        set destination 10.50.0.0/16
        set gateway 192.168.10.254
        set metric 200
    next
end
```

`edit office` creates a new route draft when that name does not exist; choose a
name yourself rather than waiting for it to appear in completion. `edit ?` lists
existing routes and explains how to create a new named route.

Within `config network route`, `show` displays only candidate route tables,
grouped by interface, with name, destination, gateway and metric columns. It does
not dump interface addresses, DNS, source filenames, internal versions or the
whole network view. Empty interfaces have an explicit empty-list message. A
selected route limits `show` to that route; `next` returns to all route tables.
Incomplete drafts receive instructions for missing fields. These are configured
Netplan `routes` entries, not DHCP/kernel-learned routes; use `execute network
status` for live state. Legacy `gateway4`/`gateway6` fields are not imported into
this table, and their interface remains read-only as described below.

Use `delete office` in the route context to remove a route. Destination must be a canonical network CIDR; gateway must be an IP of the same address family. The route's interface must be editable under policy. Several routes and several addresses per interface are supported. Existing routes receive stable displayed names; names created in SafeTerm remain available after confirmation.

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
