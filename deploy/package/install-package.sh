#!/bin/sh
set -eu
# Never snapshot/replace a daemon while a network trial is awaiting confirmation.
if [ -e /var/lib/safeterm/network/pending.json ]; then
 echo "Network trial pending: execute network confirm or execute network rollback before installation/update/rollback" >&2
 exit 1
fi

usage() {
  cat >&2 <<'EOF'
Usage:
  sudo ./install.sh                         guided fresh install or preserve existing setup
  install.sh --prepare-catalog DIR
  install.sh --signed-catalog DIR [options]
  install.sh --local-signing [options]

Install options:
  --ssh-user USER             SafeTerm SSH identity for a fresh install (default: admin)
  --ssh-password-hash-file FILE  bcrypt hash for a fresh SSH identity
  --ssh-public-key FILE       optional authorized public key
  --dry-run                   verify and print the base installation plan without host mutation
  --non-interactive          never prompt; supply bootstrap credentials for fresh installs
  --install-dependencies     install missing OS packages using configured APT repositories
  --no-start                  install but do not enable or start services
  --local-key-dir DIR         local-signing key directory (default: /root/.config/safeterm)

Production uses --prepare-catalog, signs the result on the offline signing host,
and returns that directory for --signed-catalog. Guided installation defaults to
local signing; its private signing key stays on the target under /root.
EOF
  exit "${1:-2}"
}

package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
payload="$package_dir/payload"
prepare_output=
signed_catalog=
catalog_source=
local_signing=false
ssh_user=admin
ssh_user_set=false
non_interactive=false
install_dependencies=false
bootstrap_console=false
ssh_public_key=
ssh_password_hash_file=
dry_run=false
start_services=true
local_key_dir=/root/.config/safeterm
transaction_root=/var/backups/safeterm-installer
transaction=
units='safeterm-network-manager.service safeterm-audit.service safeterm-config-writer.service safeterm-identity.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service'

stop_units_if_present() {
  for unit in "$@"; do
    # Upgrades can introduce a unit that the installed release does not have.
    # Ignore only that not-installed case; a real stop failure still aborts.
    if systemctl cat "$unit" >/dev/null 2>&1; then
      systemctl stop "$unit"
    fi
  done
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --prepare-catalog) [ "$#" -ge 2 ] || usage; prepare_output=$2; shift 2 ;;

    --signed-catalog) [ "$#" -ge 2 ] || usage; signed_catalog=$2; shift 2 ;;
    --local-signing) local_signing=true; shift ;;
    --ssh-user) [ "$#" -ge 2 ] || usage; ssh_user=$2; ssh_user_set=true; shift 2 ;;
    --ssh-password-hash-file) [ "$#" -ge 2 ] || usage; ssh_password_hash_file=$2; shift 2 ;;
    --ssh-public-key) [ "$#" -ge 2 ] || usage; ssh_public_key=$2; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    --non-interactive) non_interactive=true; shift ;;
    --install-dependencies) install_dependencies=true; shift ;;
    --no-start) start_services=false; shift ;;
    --local-key-dir) [ "$#" -ge 2 ] || usage; local_key_dir=$2; shift 2 ;;
    --help|-h) usage 0 ;;
    *) usage ;;
  esac
done

[ -f "$package_dir/PACKAGE-SHA256SUMS" ] || { echo "package checksum manifest is missing" >&2; exit 1; }
(cd "$package_dir" && sha256sum -c PACKAGE-SHA256SUMS)

case "$(uname -m)" in x86_64) host_arch=amd64 ;; aarch64|arm64) host_arch=arm64 ;; *) host_arch=unsupported ;; esac
target_arch=$(cat "$package_dir/TARGET_ARCH")
[ "$host_arch" = "$target_arch" ] || { echo "package architecture $target_arch does not match host $host_arch" >&2; exit 1; }
. "$payload/install-support.sh"

for legacy in config-manager operations policies profiles executables roles catalog.json catalog.sig resources.json .resources-manifest.json; do
  [ ! -e "/etc/safeterm/$legacy" ] && [ ! -L "/etc/safeterm/$legacy" ] || { echo "Legacy layout detected: /etc/safeterm/$legacy. Clean resources.d installation required; no import or deletion performed." >&2; exit 1; }
done
[ -n "$catalog_source" ] || catalog_source="$payload/catalog-source"

if [ -n "$prepare_output" ]; then
  [ -z "$signed_catalog" ] && [ "$local_signing" = false ] || usage
  "$payload/bin/safeterm-catalog-prepare" --source "$catalog_source" --compatible-with "$payload/catalog-source" --output "$prepare_output"
  cat <<EOF
Unsigned target catalog created at: $prepare_output
Transfer it to the offline signing host, then run:
  # On a fresh deployment only: generate its signing key pair.
  # For updates, retain the existing key matching installed trust.
  ./payload/bin/safeterm-catalog-sign -generate-key -private-key /secure/catalog.key -public-key $prepare_output/trust/catalog-ed25519.pub
  ./payload/bin/safeterm-catalog-sign -config $prepare_output -private-key /secure/catalog.key -signature $prepare_output/catalog.sig
Return the signed directory and install with:
  sudo ./install.sh --signed-catalog $prepare_output [options]
EOF
  exit 0
fi

if [ -n "$signed_catalog" ] && [ "$local_signing" = true ]; then usage; fi
if [ -z "$signed_catalog" ] && [ "$local_signing" = false ] && [ ! -f /var/lib/safeterm/generated/catalog.json ]; then
  if [ "$non_interactive" = false ] && safeterm_has_terminal; then
    echo "Fresh installation: using local catalog signing; the private key stays under $local_key_dir."
    local_signing=true
  fi
fi
if [ -z "$signed_catalog" ] && [ "$local_signing" = false ]; then
  if [ -f /usr/share/safeterm/defaults/catalog.sig ]; then signed_catalog=/usr/share/safeterm/defaults; else echo "Fresh unattended install requires --local-signing or --signed-catalog DIR and bootstrap credentials." >&2; exit 2; fi
fi
[ "$(id -u)" -eq 0 ] || { echo "installation requires root" >&2; exit 1; }
IFS= read -r init_name </proc/1/comm
[ "$init_name" = systemd ] || { echo "SafeTerm installation requires systemd running as PID 1." >&2; exit 1; }
[ ! -e /var/lib/safeterm-installer/catalog/pending ] || { echo "interrupted catalog transaction: run sudo termctl apply --recover first" >&2; exit 1; }
safeterm_dependencies
exec 9>/run/lock/safeterm-lifecycle.lock
flock -n 9 || { echo "another SafeTerm lifecycle operation is running" >&2; exit 1; }
[ ! -e /var/lib/safeterm-installer/catalog/pending ] || { echo "interrupted catalog transaction: run sudo termctl apply --recover first" >&2; exit 1; }
for tool in cmp sha256sum install useradd usermod groupadd getent systemctl ssh-keygen awk grep sed tar date find realpath visudo; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 1; }
done
getent group adm >/dev/null 2>&1 || { echo "required Ubuntu group is missing: adm" >&2; exit 1; }
case "$ssh_user" in ''|*[!A-Za-z0-9_.@-]*) echo "invalid SSH identity" >&2; exit 1 ;; esac
[ "${#ssh_user}" -le 128 ] || { echo "SSH identity is too long" >&2; exit 1; }

work=$(mktemp -d "${TMPDIR:-/tmp}/safeterm-package-install.XXXXXX")
cleanup() {
  if [ -n "${terminal_state:-}" ]; then stty "$terminal_state" 2>/dev/null || true; fi
  rm -rf -- "$work"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if [ ! -f /etc/safeterm/ssh/users.json ]; then
  safeterm_enroll
fi

local_key_dir=$(realpath -m -- "$local_key_dir")
case "$local_key_dir" in
  /root/?*) ;;
  *) echo "--local-key-dir must be an absolute child of /root" >&2; exit 1 ;;
esac

create_install_transaction() {
  transaction_id=$(date -u +%Y%m%dT%H%M%SZ)-$$
  transaction="$transaction_root/$transaction_id"
  [ ! -e "$transaction" ] || { echo "rollback transaction already exists: $transaction_id" >&2; exit 1; }
  install -d -o root -g root -m 0700 "$transaction_root" "$transaction"
  : >"$transaction/present-paths"
  : >"$transaction/absent-paths"
  : >"$transaction/replace-paths"
  : >"$transaction/service-state"

  replace_paths='/usr/local/bin/safeterm-audit
/usr/local/bin/safeterm-identity
/usr/local/bin/safeterm-config-manager
/usr/local/bin/safeterm-network-manager
/usr/local/bin/safeterm-config-writer
/usr/local/bin/safeterm-console
/usr/local/bin/safeterm-gateway
/usr/local/bin/safeterm-runner
/usr/local/bin/safeterm-ssh-broker
/usr/local/bin/safeterm-preflight
/usr/local/bin/safeterm-password-hash
/usr/local/bin/safeterm-termctl
/usr/local/bin/termctl
/usr/share/bash-completion/completions/termctl
/usr/share/safeterm/defaults
/usr/local/sbin/safeterm-install-rollback
/usr/local/sbin/safeterm-uninstall
/etc/safeterm
/etc/cat
/etc/systemd/system/safeterm-network-manager.service
/etc/systemd/system/safeterm-config-writer.service
/etc/systemd/system/safeterm-config-writer.service.d
/etc/systemd/system/safeterm-audit.service
/etc/systemd/system/safeterm-identity.service
/etc/systemd/system/safeterm-runner-diagnostics.service
/etc/systemd/system/safeterm-runner-network-observer.service
/etc/systemd/system/safeterm-runner-log-reader.service
/etc/systemd/system/safeterm-runner-service-manager.service
/etc/systemd/system/safeterm-gateway.service
/etc/systemd/system/safeterm-config-manager-application.service
/etc/systemd/system/safeterm-config-manager-database.service
/etc/systemd/system/safeterm-config-manager-network.service
/etc/systemd/system/safeterm-ssh-broker.service
/etc/sudoers.d/safeterm-service-manager
/var/lib/safeterm
/var/lib/safeterm-installer
/var/log/safeterm-readable
/var/mail/safeterm
/var/spool/mail/safeterm
/home/safeterm'
  replace_paths="$replace_paths
/etc/systemd/system/safeterm-gateway.service.d/60-site-runners.conf"
  for site_unit in /etc/systemd/system/safeterm-runner-site-*.service; do
    [ -e "$site_unit" ] || continue
    [ ! -L "$site_unit" ] && grep -qxF '# Generated by SafeTerm profile compiler.' "$site_unit" || { echo "unmanaged reserved site Runner unit" >&2; exit 1; }
    replace_paths="$replace_paths
$site_unit"
    units="$units $(basename "$site_unit")"
  done
  printf '%s\n%s\n' "$replace_paths" "$local_key_dir" >"$transaction/replace-paths"
  while IFS= read -r path; do
    [ -n "$path" ] || continue
    if [ -e "$path" ] || [ -L "$path" ]; then
      printf '%s\n' "${path#/}" >>"$transaction/present-paths"
    else
      printf '%s\n' "$path" >>"$transaction/absent-paths"
    fi
  done <"$transaction/replace-paths"
  for path in /etc/passwd /etc/shadow /etc/group /etc/gshadow /etc/subuid /etc/subgid /etc/shells /var/log/lastlog /var/log/faillog; do
    if [ -e "$path" ] || [ -L "$path" ]; then
      printf '%s\n' "${path#/}" >>"$transaction/present-paths"
    else
      printf '%s\n' "$path" >>"$transaction/absent-paths"
    fi
  done
  tar -C / -cSpf "$transaction/snapshot.tar" -T "$transaction/present-paths"
  for unit in $units; do
    enabled=$(systemctl is-enabled "$unit" 2>/dev/null || true)
    active=$(systemctl is-active "$unit" 2>/dev/null || true)
    [ -n "$enabled" ] || enabled=not-found
    [ -n "$active" ] || active=inactive
    printf '%s %s %s\n' "$unit" "$enabled" "$active" >>"$transaction/service-state"
  done
  (cd "$transaction" && sha256sum snapshot.tar present-paths absent-paths replace-paths service-state >SNAPSHOT-SHA256SUMS)
  printf '%s\n' "$transaction_id" >"$transaction_root/latest"
  install -d -o root -g root -m 0755 /usr/local/sbin
  install -o root -g root -m 0555 "$payload/rollback.sh" /usr/local/sbin/safeterm-install-rollback
  echo "Installation rollback transaction: $transaction_id"
  echo "Rollback command: sudo /usr/local/sbin/safeterm-install-rollback $transaction_id"
}

if [ "$local_signing" = true ]; then
  if [ "$catalog_source" = /usr/share/safeterm/defaults ]; then
    # Preserve the approved hashes; an application update cannot approve new OS binaries.
    "$payload/bin/safeterm-catalog-prepare" --source "$catalog_source" --compatible-with "$payload/catalog-source" --keep-hashes --verify-source --output "$work/catalog"
  else
    "$payload/bin/safeterm-catalog-prepare" --source "$catalog_source" --compatible-with "$payload/catalog-source" --output "$work/catalog"
  fi
  key_dir="$work/local-key"
  install -d -o root -g root -m 0700 "$key_dir"
  if [ -f "$local_key_dir/catalog-ed25519.key" ] && [ -f "$local_key_dir/catalog-ed25519.pub" ]; then
    install -m 0600 "$local_key_dir/catalog-ed25519.key" "$key_dir/catalog-ed25519.key"
    install -m 0644 "$local_key_dir/catalog-ed25519.pub" "$key_dir/catalog-ed25519.pub"
  else
    [ ! -e "$local_key_dir/catalog-ed25519.key" ] && [ ! -e "$local_key_dir/catalog-ed25519.pub" ] || { echo "local signing key pair is incomplete" >&2; exit 1; }
  fi
  private_key="$key_dir/catalog-ed25519.key"
  public_key="$key_dir/catalog-ed25519.pub"
  if [ ! -f "$private_key" ] || [ ! -f "$public_key" ]; then
    [ ! -e "$private_key" ] && [ ! -e "$public_key" ] || { echo "local signing key pair is incomplete" >&2; exit 1; }
    [ ! -f /etc/safeterm/trust/catalog-ed25519.pub ] || { echo "existing trust has no local key; update without --local-signing or provide --signed-catalog DIR" >&2; exit 1; }
    "$payload/bin/safeterm-catalog-sign" -generate-key -private-key "$private_key" -public-key "$public_key"
  fi
  if [ -f /etc/safeterm/trust/catalog-ed25519.pub ]; then
    cmp -s "$public_key" /etc/safeterm/trust/catalog-ed25519.pub || { echo "local signing key does not match installed trust" >&2; exit 1; }
  fi
  install -m 0644 "$public_key" "$work/catalog/trust/catalog-ed25519.pub"
  "$payload/bin/safeterm-catalog-sign" -config "$work/catalog" -private-key "$private_key" -signature "$work/catalog/catalog.sig"
  signed_catalog="$work/catalog"
fi

[ -d "$signed_catalog" ] || { echo "signed catalog directory is missing" >&2; exit 1; }
"$payload/bin/safeterm-catalog-prepare" --source "$signed_catalog" --compatible-with "$payload/catalog-source" --keep-hashes --verify-source --output "$work/config"
install -m 0644 "$signed_catalog/catalog.sig" "$work/config/catalog.sig"
install -m 0644 "$signed_catalog/trust/catalog-ed25519.pub" "$work/config/trust/catalog-ed25519.pub"
if [ -f /etc/safeterm/trust/catalog-ed25519.pub ]; then
  cmp -s "$work/config/trust/catalog-ed25519.pub" /etc/safeterm/trust/catalog-ed25519.pub || { echo "package updates cannot rotate installed catalog trust" >&2; exit 1; }
fi

mkdir -p "$work/config/ssh" "$work/config/console"
if [ -f /etc/safeterm/ssh/ssh_host_ed25519_key ]; then
  install -m 0600 /etc/safeterm/ssh/ssh_host_ed25519_key "$work/config/ssh/ssh_host_ed25519_key"
  [ ! -f /etc/safeterm/ssh/ssh_host_ed25519_key.pub ] || install -m 0644 /etc/safeterm/ssh/ssh_host_ed25519_key.pub "$work/config/ssh/ssh_host_ed25519_key.pub"
else
  ssh-keygen -q -t ed25519 -N '' -f "$work/config/ssh/ssh_host_ed25519_key"
fi

if [ -f /etc/safeterm/ssh/users.json ]; then
  install -m 0600 /etc/safeterm/ssh/users.json "$work/config/ssh/users.json"
else
  password_hash=
  if [ -n "$ssh_password_hash_file" ]; then
    [ -f "$ssh_password_hash_file" ] || { echo "password hash file is missing" >&2; exit 1; }
    password_hash=$(cat "$ssh_password_hash_file")
    [ "${#password_hash}" -eq 60 ] || { echo "invalid bcrypt password hash" >&2; exit 1; }
    printf '%s\n' "$password_hash" | LC_ALL=C grep -Eq '^\$2[ab]\$(10|11|12|13|14)\$[./A-Za-z0-9]{53}$' || { echo "invalid bcrypt password hash" >&2; exit 1; }
  fi
  keys='[]'
  if [ -n "$ssh_public_key" ]; then
    [ -f "$ssh_public_key" ] || { echo "SSH public key file is missing" >&2; exit 1; }
    ssh-keygen -lf "$ssh_public_key" >/dev/null
    key_type=$(awk 'NR==1 {print $1}' "$ssh_public_key")
    key_data=$(awk 'NR==1 {print $2}' "$ssh_public_key")
    case "$key_type" in ssh-ed25519|ecdsa-sha2-*|ssh-rsa) ;; *) echo "unsupported SSH public key" >&2; exit 1 ;; esac
    keys="[\"$key_type $key_data\"]"
  fi
  [ -n "$password_hash" ] || [ "$keys" != '[]' ] || { echo "--ssh-password-hash-file or --ssh-public-key is required for a fresh install" >&2; exit 1; }
  cat >"$work/config/ssh/users.json" <<EOF
{"users":[{"user_id":"$ssh_user","roles":["viewer","netops","log-viewer","service-viewer","config-viewer","config-operator","config-approver"],"password_hash":"$password_hash","authorized_keys":$keys}]}
EOF
  chmod 0600 "$work/config/ssh/users.json"
fi

if [ -f /etc/safeterm/console/users.json ]; then
  install -m 0600 /etc/safeterm/console/users.json "$work/config/console/users.json"
elif [ "$bootstrap_console" = true ]; then
  password_hash=$(cat "$ssh_password_hash_file")
  cat >"$work/config/console/users.json" <<EOF
{"users":[{"user_id":"$ssh_user","roles":["viewer","netops","log-viewer","service-viewer","config-viewer","config-operator"],"password_hash":"$password_hash"}]}
EOF
  chmod 0600 "$work/config/console/users.json"
else
  install -m 0600 "$payload/console/users.json" "$work/config/console/users.json"
fi
install -m 0644 "$payload/resources.json" "$work/config/resources.json"


mkdir -p "$work/bundle/bin" "$work/bundle/etc" "$work/bundle/managed" "$work/bundle/systemd"
for name in audit network-manager config-manager config-writer console gateway runner ssh-broker preflight termctl password-hash identity; do
  install -m 0555 "$payload/bin/safeterm-$name" "$work/bundle/bin/safeterm-$name"
done
cp -R "$work/config" "$work/bundle/etc/safeterm"
cp "$payload/catalog-source/README.md" "$payload/catalog-source/README.txt" "$work/bundle/etc/safeterm/"
cp "$payload/catalog-source/NETWORK.md" "$payload/catalog-source/NETWORK.txt" "$payload/catalog-source/NETWORK-DIAGNOSTICS.md" "$payload/catalog-source/NETWORK-DIAGNOSTICS.txt" "$payload/catalog-source/network.json" "$work/bundle/etc/safeterm/"
for definitions in resources.d roles.d profiles.d operations.d executables.d policies.d; do
 mkdir -p "$work/bundle/etc/safeterm/$definitions"
 cp "$payload/catalog-source/$definitions/TEMPLATE.json" "$payload/catalog-source/$definitions/README.md" "$payload/catalog-source/$definitions/README.txt" "$work/bundle/etc/safeterm/$definitions/"
done
cp -R "$payload/managed/." "$work/bundle/managed/"
cp "$payload/systemd/"*.service "$work/bundle/systemd/"
chmod 0644 "$work/bundle/systemd/"*.service
chmod 0600 "$work/bundle/managed/"*/*
"$work/bundle/bin/safeterm-preflight" --bundle "$work/bundle" --stage-managed-upgrade
(cd "$work/bundle" && find bin etc managed systemd -type f -print | LC_ALL=C sort | xargs sha256sum >SHA256SUMS)
chmod 0444 "$work/bundle/SHA256SUMS"
prior_identity_catalog=
if [ -f /var/lib/safeterm/generated/catalog.sig ]; then
 cp -R /var/lib/safeterm/generated "$work/prior-active-catalog"
 prior_identity_catalog="$work/prior-active-catalog"
fi
"$work/bundle/bin/safeterm-preflight" --bundle "$work/bundle" --prior-identity-catalog "$prior_identity_catalog"

if [ "$dry_run" = true ]; then
  "$payload/base-install.sh" --bundle "$work/bundle" --dry-run
  echo "SafeTerm package dry run: PASS"
  exit 0
fi

create_install_transaction
# Reference defaults are versioned separately from the active site configuration.
install -d -o root -g root -m 0755 /usr/share/safeterm
if [ "$local_signing" = true ]; then
  install -d -o root -g root -m 0700 "$local_key_dir"
  install -o root -g root -m 0600 "$private_key" "$local_key_dir/catalog-ed25519.key"
  install -o root -g root -m 0644 "$public_key" "$local_key_dir/catalog-ed25519.pub"
  echo "WARNING: local signing key retained at $local_key_dir/catalog-ed25519.key; use offline signing for production."
fi

if [ -f /etc/systemd/system/safeterm-ssh-broker.service ]; then
  for site_unit in /etc/systemd/system/safeterm-runner-site-*.service; do
    [ -f "$site_unit" ] || continue
    stop_units_if_present "$(basename "$site_unit")"
  done
  stop_units_if_present safeterm-ssh-broker.service
  stop_units_if_present safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service
  stop_units_if_present safeterm-gateway.service
  stop_units_if_present safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service
  stop_units_if_present safeterm-network-manager.service
  stop_units_if_present safeterm-config-writer.service
  stop_units_if_present safeterm-identity.service
  stop_units_if_present safeterm-audit.service
fi
"$payload/base-install.sh" --bundle "$work/bundle" --no-start
# Keep the administrator entry point explicit even if a custom base installer
# is supplied with a locally rebuilt package.
install -o root -g root -m 0555 "$work/bundle/bin/safeterm-termctl" /usr/local/bin/safeterm-termctl
install -o root -g root -m 0555 "$work/bundle/bin/safeterm-termctl" /usr/local/bin/termctl
install -o root -g root -m 0555 "$payload/uninstall.sh" /usr/local/sbin/safeterm-uninstall
[ -x /usr/local/bin/safeterm-termctl ] || { echo "termctl installation failed" >&2; exit 1; }
[ -x /usr/local/bin/termctl ] || { echo "termctl alias installation failed" >&2; exit 1; }
if [ "$local_signing" = true ]; then
  /usr/local/bin/termctl initialize --installer-lock-held --key-file "$local_key_dir/catalog-ed25519.key"
elif [ -n "$prior_identity_catalog" ]; then
  # Reuse only an already trusted signature whose model exactly matches the
  # newly compiled sources. No prior definition is imported as source.
  /usr/local/bin/termctl initialize --installer-lock-held --signed-catalog "$prior_identity_catalog"
else
  /usr/local/bin/termctl initialize --installer-lock-held
fi
/usr/local/bin/safeterm-preflight --installed
if [ "$start_services" = true ]; then
  systemctl enable safeterm-network-manager.service safeterm-config-writer.service safeterm-identity.service safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
  systemctl restart safeterm-audit.service
  systemctl restart safeterm-config-writer.service
  systemctl restart safeterm-identity.service
  systemctl restart safeterm-network-manager.service
  systemctl restart safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service
  systemctl restart safeterm-gateway.service
  systemctl restart safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service
  systemctl restart safeterm-ssh-broker.service
  systemctl --quiet is-active safeterm-network-manager.service safeterm-config-writer.service safeterm-identity.service safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
fi
echo "SafeTerm package installation: PASS"
# Retain one pre-install snapshot; operators never need to track IDs and
# repeated upgrades do not accumulate snapshots indefinitely.
for old_transaction in "$transaction_root"/*; do
  [ -d "$old_transaction" ] || continue
  [ "$old_transaction" = "$transaction" ] || rm -rf -- "$old_transaction"
done
echo "Administration: sudo termctl rollback | sudo termctl uninstall"
echo "Rollback remains available with: sudo /usr/local/sbin/safeterm-install-rollback $(basename "$transaction")"

if [ "$bootstrap_console" = true ]; then
  echo "SSH login: ssh -tt -p 2222 $ssh_user@SERVER_IP"
  echo "Console login: sudo -iu safeterm, then enter username $ssh_user and the password you set."
  echo "SSH and Console credentials are independent after enrollment."
fi
