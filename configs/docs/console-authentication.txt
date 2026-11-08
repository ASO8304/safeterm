# Console login through Linux

Create each operator's new Linux account from an ordinary Linux administrator
shell with one command:

```sh
sudo termctl user add console --os-user alice --roles viewer,netops,config-viewer,config-operator
```

Choose an unused username. The command refuses existing users, same-name groups
and existing home paths without adopting or changing them. It asks for the new
Linux password twice without echo, creates the home/private primary group, grants
only the SafeTerm socket groups, assigns the SafeTerm login shell and saves the
requested role mapping. --id is optional and defaults to --os-user.

SafeTerm services restart to load the new UID; active sessions must reconnect.
Provisioning is blocked during an active network trial or lifecycle operation.
The new account stays password-locked until access is ready. Failure removes its
new mapping and leaves the new account locked; home data is never deleted.

Log in to Linux as alice with that Linux password. SafeTerm opens directly,
without a second login prompt. An administrator can test entry with sudo -iu alice.
Keep a separate Linux maintenance account; no sudo or unrelated privileged groups
are granted. Existing older Console mappings remain supported without changing
their Linux accounts. passwd inside SafeTerm changes the current login password.

The fresh installer enrolls SSH and Linux Console users in separate steps. It
does not copy SSH passwords into a Console store. Console enrollment is optional
and requires an unused Linux username. Upgrades preserve existing identities;
unattended installs defer Console provisioning to the command above.

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
