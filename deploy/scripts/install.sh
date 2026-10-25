#!/bin/sh
set -eu

usage() {
  echo "usage: $0 --bundle DIR [--dry-run] [--no-start]" >&2
  exit 2
}

bundle=
dry_run=false
start_services=true
signing_key=/root/.config/safeterm/catalog-ed25519.key
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle) [ "$#" -ge 2 ] || usage; bundle=$2; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    --no-start) start_services=false; shift ;;
    --signing-key) [ "$#" -ge 2 ] || usage; signing_key=$2; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$bundle" ] && [ -d "$bundle" ] || usage
for legacy in config-manager operations policies profiles executables roles catalog.json catalog.sig resources.json .resources-manifest.json; do
  [ ! -e "/etc/safeterm/$legacy" ] && [ ! -L "/etc/safeterm/$legacy" ] || { echo "Legacy administrator layout is unsupported; use a clean resources.d installation." >&2; exit 1; }
done
if [ "$start_services" = true ] && [ "$dry_run" = false ] && [ -e /var/lib/safeterm/generated/catalog.json ]; then
  echo "Use the transactional release installer for updates." >&2; exit 1
fi
bundle=$(CDPATH= cd -- "$bundle" && pwd)

for tool in sha256sum install groupadd useradd usermod getent systemctl visudo; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 1; }
done
if find "$bundle" -type l -print -quit | grep -q .; then
  echo "bundle must not contain symlinks" >&2
  exit 1
fi
[ -f "$bundle/SHA256SUMS" ] || { echo "bundle checksum manifest is missing" >&2; exit 1; }
(cd "$bundle" && sha256sum -c SHA256SUMS)
work=$(mktemp -d "${TMPDIR:-/tmp}/safeterm-install.XXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM
prior_identity_catalog=
if [ "$(id -u)" -eq 0 ] && [ -f /var/lib/safeterm/generated/catalog.sig ]; then
 cp -R /var/lib/safeterm/generated "$work/prior-identity-catalog"
 prior_identity_catalog="$work/prior-identity-catalog"
fi
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --prior-identity-catalog "$prior_identity_catalog"
sudoers_plan="$work/safeterm-service-manager"
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --prior-identity-catalog "$prior_identity_catalog" --sudoers-plan >"$sudoers_plan"
chmod 0440 "$sudoers_plan"
visudo -cf "$sudoers_plan" >/dev/null
writer_plan="$work/writer-paths.conf"
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --prior-identity-catalog "$prior_identity_catalog" --writer-plan >"$writer_plan"

cat <<EOF
SafeTerm deployment plan
  bundle:       $bundle
  binaries:     /usr/local/bin/safeterm-*
  configuration:/etc/safeterm
  units:        /etc/systemd/system/safeterm-*.service
  managed data: /etc/safeterm-managed
  start:        $start_services
EOF
[ "$dry_run" = false ] || { echo "Dry run: PASS"; exit 0; }
[ "$(id -u)" -eq 0 ] || { echo "installation requires root" >&2; exit 1; }
if [ "$start_services" = true ]; then
  exec 9>/run/lock/safeterm-lifecycle.lock
  flock -n 9 || { echo "another SafeTerm lifecycle operation is running" >&2; exit 1; }
fi
getent group adm >/dev/null 2>&1 || { echo "required Ubuntu group is missing: adm" >&2; exit 1; }

groups='safeterm-config-writers safeterm safeterm-identity safeterm-audit safeterm-broker safeterm-clients safeterm-gateway safeterm-diagnostics safeterm-network safeterm-log-reader safeterm-service-manager safeterm-config-application safeterm-config-database safeterm-config-network'
for name in $groups; do
  groupadd -f "$name"
  [ "$name" = safeterm ] && continue
  if ! id -u "$name" >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin --gid "$name" "$name"
  fi
done

if ! id -u safeterm >/dev/null 2>&1; then
  useradd --create-home --shell /usr/local/bin/safeterm-console --gid safeterm safeterm
fi

for name in safeterm-identity safeterm-gateway safeterm-diagnostics safeterm-network safeterm-log-reader safeterm-service-manager safeterm-config-application safeterm-config-database safeterm-config-network; do
  usermod -a -G safeterm-audit "$name"
done
usermod -a -G adm safeterm-log-reader
usermod -a -G safeterm-clients safeterm-gateway
usermod -a -G safeterm-gateway,safeterm-clients safeterm-broker
usermod -a -G safeterm-gateway,safeterm-clients safeterm
for name in safeterm-config-application safeterm-config-database safeterm-config-network; do
  usermod -a -G "safeterm-gateway,safeterm-config-writers,$name" "$name"
done

for binary in "$bundle"/bin/safeterm-*; do
  install -o root -g root -m 0555 "$binary" "/usr/local/bin/$(basename -- "$binary")"
done
install -o root -g root -m 0555 "$bundle/bin/safeterm-termctl" /usr/local/bin/termctl
install -d -o root -g root -m 0755 /usr/share/bash-completion/completions
"$bundle/bin/safeterm-termctl" completion bash > /usr/share/bash-completion/completions/termctl
chown root:root /usr/share/bash-completion/completions/termctl
chmod 0644 /usr/share/bash-completion/completions/termctl
usermod --shell /usr/local/bin/safeterm-console safeterm
if ! grep -qxF /usr/local/bin/safeterm-console /etc/shells; then
  echo /usr/local/bin/safeterm-console >>/etc/shells
fi
install -d -o root -g root -m 0755 /etc/safeterm /var/lib/safeterm /var/lib/safeterm/generated
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --prior-identity-catalog "$prior_identity_catalog" --install-configuration
chown root:safeterm-broker /etc/safeterm/ssh/ssh_host_ed25519_key /etc/safeterm/ssh/users.json
chmod 0640 /etc/safeterm/ssh/ssh_host_ed25519_key /etc/safeterm/ssh/users.json
chown root:safeterm-identity /etc/safeterm/console/users.json
chmod 0640 /etc/safeterm/console/users.json

[ -d /etc/safeterm-managed ] || install -d -o root -g root -m 0755 /etc/safeterm-managed
managed_plan="$work/managed-plan"
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --prior-identity-catalog "$prior_identity_catalog" --managed-plan >"$managed_plan"
tab=$(printf '\t')
while IFS="$tab" read -r source target owner; do
  [ -n "$source" ] && [ -n "$target" ] && [ -n "$owner" ] || { echo "invalid managed install plan" >&2; exit 1; }
  if [ -e "$target" ] || [ -L "$target" ]; then continue; fi
  if [ ! -d "$(dirname -- "$target")" ]; then
    install -d -o "$owner" -g "$owner" -m 0700 "$(dirname -- "$target")"
  fi
  install -o "$owner" -g "$owner" -m 0600 "$source" "$target"
done <"$managed_plan"
rm -f -- "$managed_plan"
[ -d /etc/cat ] || install -d -o root -g root -m 0750 /etc/cat
install -d -o root -g root -m 0755 /etc/sudoers.d
install -o root -g root -m 0440 "$sudoers_plan" /etc/sudoers.d/safeterm-service-manager

for unit in "$bundle"/systemd/*.service; do
  install -o root -g root -m 0644 "$unit" "/etc/systemd/system/$(basename -- "$unit")"
done
install -d -o root -g root -m 0755 /etc/systemd/system/safeterm-config-writer.service.d
install -o root -g root -m 0644 "$writer_plan" /etc/systemd/system/safeterm-config-writer.service.d/50-resources.conf
systemctl daemon-reload
if [ "$start_services" = true ]; then
  /usr/local/bin/termctl initialize --installer-lock-held --key-file "$signing_key"
  /usr/local/bin/safeterm-preflight --installed
fi
if [ "$start_services" = true ]; then
  systemctl enable safeterm-config-writer.service safeterm-identity.service safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
  systemctl restart safeterm-audit.service
  systemctl restart safeterm-config-writer.service
  systemctl restart safeterm-identity.service
  systemctl restart safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service
  systemctl restart safeterm-gateway.service
  systemctl restart safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service
  systemctl restart safeterm-ssh-broker.service
  systemctl --quiet is-active safeterm-identity.service safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
fi
echo "SafeTerm installation: PASS"
