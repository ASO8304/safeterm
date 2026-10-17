#!/bin/sh
# Sourced only after the package manifest has been verified. No host mutation
# takes place merely by sourcing this file.

safeterm_has_terminal() { [ -t 0 ]; }

safeterm_find_missing() {
  missing=
  for tool in stty install find awk grep sed tar date realpath cmp flock useradd usermod groupadd getent systemctl ssh-keygen visudo; do
    command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
  done
  for executable in /usr/bin/uname /usr/bin/tcpdump /usr/bin/tail /usr/bin/systemctl /usr/bin/sudo /usr/bin/ping /usr/bin/nslookup /usr/bin/traceroute /usr/bin/free /usr/bin/df; do
    [ -x "$executable" ] || missing="$missing $executable"
  done
}

safeterm_dependencies() {
  safeterm_find_missing
  [ -n "$missing" ] || return 0
  if [ "$dry_run" = true ]; then
    echo "Missing OS prerequisites:$missing. Dry run does not install OS packages." >&2
    return 1
  fi
  if [ "$install_dependencies" != true ]; then
    if [ "$non_interactive" = true ] || ! safeterm_has_terminal; then
      echo "Missing OS prerequisites:$missing. Provision them offline or pass --install-dependencies to use APT." >&2
      return 1
    fi
  fi
  [ -r /etc/os-release ] || { echo "Cannot identify OS; provision prerequisites manually." >&2; return 1; }
  ID=
  . /etc/os-release
  case "$ID" in ubuntu|debian) ;; *) echo "Automatic prerequisite installation supports Ubuntu/Debian only." >&2; return 1 ;; esac
  command -v apt-get >/dev/null 2>&1 || { echo "APT is unavailable; provision prerequisites offline." >&2; return 1; }
  echo "Installing missing OS prerequisites from configured APT repositories:$missing"
  DEBIAN_FRONTEND=noninteractive apt-get update || { echo "APT repository access failed; provision prerequisites offline and rerun install.sh." >&2; return 1; }
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-upgrade ca-certificates coreutils findutils gawk grep gzip \
    libc-bin openssh-client passwd sed sudo systemd tar tcpdump util-linux \
    iputils-ping dnsutils traceroute procps || {
      echo "OS package installation failed. For an offline server provision prerequisites from your internal mirror/media and rerun install.sh." >&2
      return 1
    }
  safeterm_find_missing
  [ -z "$missing" ] || { echo "Required commands still missing after APT:$missing" >&2; return 1; }
}

safeterm_enroll() {
  # Explicit hash/key options remain the unattended installation contract.
  [ -z "$ssh_password_hash_file" ] && [ -z "$ssh_public_key" ] || return 0
  if [ "$non_interactive" = true ] || ! safeterm_has_terminal; then
    echo "Fresh installation needs credentials. Run sudo ./install.sh in a terminal, or supply --ssh-password-hash-file / --ssh-public-key for automation." >&2
    return 1
  fi
  terminal_state=$(stty -g 2>/dev/null || true)
  echo "Create the initial SafeTerm user for SSH and Console (no separate Linux user)." >&2
  if [ "$ssh_user_set" = false ]; then
    printf 'SafeTerm username [admin]: ' >&2
    IFS= read -r answer || return 1
    ssh_user=${answer:-admin}
  fi
  case "$ssh_user" in ''|*[!A-Za-z0-9_.@-]*) echo "Invalid SafeTerm username." >&2; return 1 ;; esac
  [ "${#ssh_user}" -le 128 ] || { echo "SafeTerm username is too long." >&2; return 1; }
  echo "Password: 8-72 printable ASCII characters, uppercase, lowercase and a number." >&2
  ssh_password_hash_file="$work/bootstrap.hash"
  # The bundled helper reads the terminal with echo disabled, confirms the
  # password, validates complexity and creates only a mode-0600 bcrypt hash.
  while ! "$payload/bin/safeterm-password-hash" -output "$ssh_password_hash_file"; do
    rm -f -- "$ssh_password_hash_file"
    printf 'Try again? [Y/n]: ' >&2
    IFS= read -r answer || return 1
    case "$answer" in n|N|no|NO) return 1 ;; esac
  done
  bootstrap_console=true
}
