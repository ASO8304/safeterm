# SSH authentication

SafeTerm SSH accepts usernames and passwords using `password_hash` in
`/etc/safeterm/ssh/users.json`. Passwords are independent of Linux account
passwords. Public-key login is optional: add `authorized_keys` only for users
who need it. Either configured method authenticates the same server-owned roles;
configuring both allows either method, not two-factor authentication.

## Generate a hash

The offline package includes a utility; no Go or Internet access is required:

```sh
./payload/bin/safeterm-password-hash -output /secure/alice.hash
```

It prompts twice with echo disabled, creates a new mode-0600 file, and refuses
to overwrite an existing file. Installed packages also provide
`/usr/local/bin/safeterm-password-hash`. Passwords are salted with bcrypt at cost
12 and must contain 1–72 bytes (UTF-8 characters can occupy multiple bytes).
Passwords are never command-line arguments. For automation, `-stdin` accepts a
single password line from a protected pipe; avoid putting secrets in shell history.

## Fresh package installation

```sh
sudo ./install.sh --local-signing --ssh-user alice \
  --ssh-password-hash-file /secure/alice.hash --dry-run
sudo ./install.sh --local-signing --ssh-user alice \
  --ssh-password-hash-file /secure/alice.hash
```

Add `--ssh-public-key /secure/alice.pub` only if desired. Offline signing uses
the same credential options with `--signed-catalog DIR` instead.

For Ansible, configure:

```yaml
safeterm_bootstrap_ssh_user: alice
safeterm_bootstrap_ssh_password_hash_file: /secure/alice.hash
safeterm_bootstrap_ssh_public_key_file: ""
```

Then run `deploy/ansible/install.sh`. Never put a plaintext password in inventory,
variables, JSON, source control, or the chat.

## Existing installation and user configuration

Package upgrades preserve identity files. To enable a password for an existing
user, replace that user's `password_hash` with the generated hash in the
root-managed identity map and restart `safeterm-ssh-broker.service`. Remove
`authorized_keys` to make that user password-only. Keep the file root-owned,
group `safeterm-broker`, mode 0640. Example structure (replace the placeholder):

```json
{
  "users": [{
    "user_id": "alice",
    "roles": ["viewer", "config-viewer"],
    "password_hash": "$2a$12$REPLACE_WITH_GENERATED_BCRYPT_HASH"
  }]
}
```

With Ansible, define the complete identity list; roles here are illustrative:

```yaml
safeterm_ssh_users:
  - user_id: alice
    roles: [viewer, config-viewer]
    password_hash: "{{ lookup('file', '/secure/alice.hash') | trim }}"
```

The managed list replaces the entire identity file. An empty list preserves
existing users. A user must have roles and at least one valid credential.
Key-only users from older installations remain supported. Plaintext passwords,
malformed hashes, unsupported bcrypt prefixes, and costs outside 10–14 are
rejected before the broker starts. A bad configured hash also fails validation
when the user has valid public keys; it is never silently ignored.

## Connect

```sh
ssh -t -p 2222 alice@SERVER_IP
```

Enter the password in the SSH client's prompt. To explicitly test password-only
login even when your SSH agent has keys:

```sh
ssh -t -p 2222 -o PubkeyAuthentication=no \
  -o PreferredAuthentications=password alice@SERVER_IP
```

The connection still opens only SafeTerm's restricted terminal. Password login
does not grant a Linux shell, SSH exec, file transfer, or port forwarding.
