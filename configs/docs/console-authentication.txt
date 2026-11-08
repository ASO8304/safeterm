# Console login through Linux

Map each operator's existing Linux login to a SafeTerm identity and roles. From
an ordinary Linux administrator account:

```sh
sudo usermod -a -G safeterm-clients,safeterm-gateway alice
sudo termctl user add console --id alice --os-user alice --roles viewer,netops,config-viewer,config-operator
sudo termctl service restart all
```

Choose roles for the operator's responsibilities. Log out of Linux and log back
in to load new group memberships, then run `termctl` without sudo. SafeTerm opens
without a second login prompt. For a restricted account that enters SafeTerm
directly, set its Linux shell explicitly:

```sh
sudo usermod --shell /usr/local/bin/safeterm-console alice
```

Keep a separate Linux maintenance account. SafeTerm does not create accounts,
set Linux passwords or change groups/shells automatically. Linux manages the
password with `passwd`.

Mappings live in `/etc/safeterm/access/console/linux-users.json`, root-owned mode
0644. The default is an empty `users` array. A record has `os_user`, `user_id`,
`roles`, and optional `disabled: true`. Mapping removal and role changes revoke
sessions at the next verification; streams recheck within 15 seconds. Adding a
Linux UID requires restarting SafeTerm services. Never recycle an account/UID
while its mapping remains enabled. Root and SafeTerm service accounts cannot be
mapped. Kernel UID/PID identifies the Console process; environment variables and
client-supplied role claims do not authenticate it.

```sh
sudo termctl user list
sudo termctl user auth console --id alice --os-user alice --roles viewer,config-viewer
sudo termctl user del console --id alice --os-user alice
```

The legacy shared account still supports `sudo -iu safeterm` followed by an
internal SafeTerm Username/Password for upgrades. Its users remain in the
separate `access/console/users.json` file. Old `os_user` fields in that credential
file do not grant Linux login access. SSH authentication is unchanged.
