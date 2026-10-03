#!/bin/sh
set -eu

usage() {
  cat >&2 <<'EOF'
Usage:
  install.sh --prepare-catalog DIR
  install.sh --signed-catalog DIR [options]
  install.sh --local-signing [options]

Install options:
  --ssh-user USER             SafeTerm SSH identity for a fresh install (default: alice)
  --ssh-public-key FILE       authorized public key; optional when upgrading an existing install
  --single-operator           let proposing roles apply their own config changes
  --dry-run                   verify and print the base installation plan without host mutation
  --no-start                  install but do not enable or start services
  --local-key-dir DIR         local-signing key directory (default: /root/.config/safeterm)

Production uses --prepare-catalog, signs the result on the offline signing host,
and returns that directory for --signed-catalog. --local-signing is intended for
demos and single-host evaluation because its private signing key stays on target.
EOF
  exit 2
}

package_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
payload="$package_dir/payload"
prepare_output=
signed_catalog=
local_signing=false
ssh_user=alice
ssh_public_key=
single_operator=false
dry_run=false
start_services=true
local_key_dir=/root/.config/safeterm

while [ "$#" -gt 0 ]; do
  case "$1" in
    --prepare-catalog) [ "$#" -ge 2 ] || usage; prepare_output=$2; shift 2 ;;
    --signed-catalog) [ "$#" -ge 2 ] || usage; signed_catalog=$2; shift 2 ;;
    --local-signing) local_signing=true; shift ;;
    --ssh-user) [ "$#" -ge 2 ] || usage; ssh_user=$2; shift 2 ;;
    --ssh-public-key) [ "$#" -ge 2 ] || usage; ssh_public_key=$2; shift 2 ;;
    --single-operator) single_operator=true; shift ;;
    --dry-run) dry_run=true; shift ;;
    --no-start) start_services=false; shift ;;
    --local-key-dir) [ "$#" -ge 2 ] || usage; local_key_dir=$2; shift 2 ;;
    --help|-h) usage ;;
    *) usage ;;
  esac
done

[ -f "$package_dir/PACKAGE-SHA256SUMS" ] || { echo "package checksum manifest is missing" >&2; exit 1; }
(cd "$package_dir" && sha256sum -c PACKAGE-SHA256SUMS)

case "$(uname -m)" in x86_64) host_arch=amd64 ;; aarch64|arm64) host_arch=arm64 ;; *) host_arch=unsupported ;; esac
target_arch=$(cat "$package_dir/TARGET_ARCH")
[ "$host_arch" = "$target_arch" ] || { echo "package architecture $target_arch does not match host $host_arch" >&2; exit 1; }
for executable in /usr/bin/uname /usr/bin/tcpdump /usr/bin/systemctl; do
  [ -x "$executable" ] || { echo "required target executable is missing: $executable" >&2; exit 1; }
done

if [ -n "$prepare_output" ]; then
  [ -z "$signed_catalog" ] && [ "$local_signing" = false ] || usage
  "$payload/bin/safeterm-catalog-prepare" --source "$payload/catalog-source" --output "$prepare_output"
  cat <<EOF
Unsigned target catalog created at: $prepare_output
Transfer it to the offline signing host, then run:
  ./payload/bin/safeterm-catalog-sign -generate-key -private-key /secure/catalog.key -public-key $prepare_output/trust/catalog-ed25519.pub
  ./payload/bin/safeterm-catalog-sign -config $prepare_output -private-key /secure/catalog.key -signature $prepare_output/catalog.sig
Return the signed directory and install with:
  sudo ./install.sh --signed-catalog $prepare_output [options]
EOF
  exit 0
fi

if [ -n "$signed_catalog" ] && [ "$local_signing" = true ]; then usage; fi
if [ -z "$signed_catalog" ] && [ "$local_signing" = false ]; then usage; fi
[ "$(id -u)" -eq 0 ] || { echo "installation requires root" >&2; exit 1; }
for tool in sha256sum install useradd usermod groupadd systemctl ssh-keygen awk grep sed; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 1; }
done
case "$ssh_user" in ''|*[!A-Za-z0-9_.@-]*) echo "invalid SSH identity" >&2; exit 1 ;; esac
[ "${#ssh_user}" -le 128 ] || { echo "SSH identity is too long" >&2; exit 1; }

work=$(mktemp -d "${TMPDIR:-/tmp}/safeterm-package-install.XXXXXX")
cleanup() { rm -rf -- "$work"; }
trap cleanup EXIT HUP INT TERM

if [ "$local_signing" = true ]; then
  "$payload/bin/safeterm-catalog-prepare" --source "$payload/catalog-source" --output "$work/catalog"
  if [ "$dry_run" = true ]; then
    key_dir="$work/local-key"
  else
    key_dir=$local_key_dir
  fi
  install -d -o root -g root -m 0700 "$key_dir"
  private_key="$key_dir/catalog-ed25519.key"
  public_key="$key_dir/catalog-ed25519.pub"
  if [ ! -f "$private_key" ] || [ ! -f "$public_key" ]; then
    [ ! -e "$private_key" ] && [ ! -e "$public_key" ] || { echo "local signing key pair is incomplete" >&2; exit 1; }
    "$payload/bin/safeterm-catalog-sign" -generate-key -private-key "$private_key" -public-key "$public_key"
  fi
  install -m 0644 "$public_key" "$work/catalog/trust/catalog-ed25519.pub"
  "$payload/bin/safeterm-catalog-sign" -config "$work/catalog" -private-key "$private_key" -signature "$work/catalog/catalog.sig"
  signed_catalog="$work/catalog"
  echo "WARNING: local signing key retained at $private_key; use offline signing for production."
fi

[ -d "$signed_catalog" ] || { echo "signed catalog directory is missing" >&2; exit 1; }
cp -R "$signed_catalog" "$work/config"

mkdir -p "$work/config/ssh" "$work/config/console" "$work/config/config-manager"
if [ -f /etc/safeterm/ssh/ssh_host_ed25519_key ]; then
  install -m 0600 /etc/safeterm/ssh/ssh_host_ed25519_key "$work/config/ssh/ssh_host_ed25519_key"
  [ ! -f /etc/safeterm/ssh/ssh_host_ed25519_key.pub ] || install -m 0644 /etc/safeterm/ssh/ssh_host_ed25519_key.pub "$work/config/ssh/ssh_host_ed25519_key.pub"
else
  ssh-keygen -q -t ed25519 -N '' -f "$work/config/ssh/ssh_host_ed25519_key"
fi

if [ -f /etc/safeterm/ssh/users.json ]; then
  install -m 0600 /etc/safeterm/ssh/users.json "$work/config/ssh/users.json"
else
  [ -n "$ssh_public_key" ] && [ -f "$ssh_public_key" ] || { echo "--ssh-public-key is required for a fresh install" >&2; exit 1; }
  ssh-keygen -lf "$ssh_public_key" >/dev/null
  key_type=$(awk 'NR==1 {print $1}' "$ssh_public_key")
  key_data=$(awk 'NR==1 {print $2}' "$ssh_public_key")
  case "$key_type" in ssh-ed25519|ecdsa-sha2-*|ssh-rsa) ;; *) echo "unsupported SSH public key" >&2; exit 1 ;; esac
  cat >"$work/config/ssh/users.json" <<EOF
{"users":[{"user_id":"$ssh_user","roles":["viewer","netops","config-viewer","config-operator","config-approver"],"authorized_keys":["$key_type $key_data"]}]}
EOF
  chmod 0600 "$work/config/ssh/users.json"
fi

if [ -f /etc/safeterm/console/users.json ]; then
  install -m 0600 /etc/safeterm/console/users.json "$work/config/console/users.json"
else
  install -m 0600 "$payload/console/users.json" "$work/config/console/users.json"
fi
if [ -f /etc/safeterm/config-manager/resources.json ]; then
  install -m 0644 /etc/safeterm/config-manager/resources.json "$work/config/config-manager/resources.json"
else
  install -m 0644 "$payload/config-manager/resources.json" "$work/config/config-manager/resources.json"
fi
if [ "$single_operator" = true ]; then
  sed -i \
    -e 's/"require_separate_approval": true/"require_separate_approval": false/g' \
    -e 's/"apply_roles": \["config-approver"\]/"apply_roles": ["config-operator"]/g' \
    "$work/config/config-manager/resources.json"
fi

mkdir -p "$work/bundle/bin" "$work/bundle/etc" "$work/bundle/managed" "$work/bundle/systemd"
for name in audit config-manager console gateway runner ssh-broker preflight; do
  install -m 0555 "$payload/bin/safeterm-$name" "$work/bundle/bin/safeterm-$name"
done
cp -R "$work/config" "$work/bundle/etc/safeterm"
cp -R "$payload/managed/." "$work/bundle/managed/"
cp "$payload/systemd/"*.service "$work/bundle/systemd/"
chmod 0644 "$work/bundle/systemd/"*.service
chmod 0600 "$work/bundle/managed/"*/*
(cd "$work/bundle" && find bin etc managed systemd -type f -print | LC_ALL=C sort | xargs sha256sum >SHA256SUMS)
chmod 0444 "$work/bundle/SHA256SUMS"
"$work/bundle/bin/safeterm-preflight" --bundle "$work/bundle"

if [ "$dry_run" = true ]; then
  "$payload/base-install.sh" --bundle "$work/bundle" --dry-run
  echo "SafeTerm package dry run: PASS"
  exit 0
fi

if [ -d /etc/safeterm-managed ]; then
  cp -a /etc/safeterm-managed "$work/preserved-managed"
fi
if [ -f /etc/systemd/system/safeterm-ssh-broker.service ]; then
  systemctl stop safeterm-ssh-broker.service
  systemctl stop safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service
  systemctl stop safeterm-gateway.service
  systemctl stop safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-service-manager.service
  systemctl stop safeterm-audit.service
fi
"$payload/base-install.sh" --bundle "$work/bundle" --no-start
if [ -d "$work/preserved-managed" ]; then
  cp -a "$work/preserved-managed/." /etc/safeterm-managed/
fi
/usr/local/bin/safeterm-preflight --installed

if [ "$start_services" = true ]; then
  systemctl enable safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
  systemctl restart safeterm-audit.service
  systemctl restart safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-service-manager.service
  systemctl restart safeterm-gateway.service
  systemctl restart safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service
  systemctl restart safeterm-ssh-broker.service
  systemctl --quiet is-active safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
fi
echo "SafeTerm package installation: PASS"
