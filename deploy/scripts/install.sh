#!/bin/sh
set -eu

usage() {
  echo "usage: $0 --bundle DIR [--dry-run] [--no-start]" >&2
  exit 2
}

bundle=
dry_run=false
start_services=true
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle) [ "$#" -ge 2 ] || usage; bundle=$2; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    --no-start) start_services=false; shift ;;
    *) usage ;;
  esac
done
[ -n "$bundle" ] && [ -d "$bundle" ] || usage
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
"$bundle/bin/safeterm-preflight" --bundle "$bundle"
work=$(mktemp -d "${TMPDIR:-/tmp}/safeterm-install.XXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM
sudoers_plan="$work/safeterm-service-manager"
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --sudoers-plan >"$sudoers_plan"
chmod 0440 "$sudoers_plan"
visudo -cf "$sudoers_plan" >/dev/null

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
getent group adm >/dev/null 2>&1 || { echo "required Ubuntu group is missing: adm" >&2; exit 1; }

groups='safeterm safeterm-audit safeterm-broker safeterm-clients safeterm-gateway safeterm-diagnostics safeterm-network safeterm-log-reader safeterm-service-manager safeterm-config-application safeterm-config-database safeterm-config-network'
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

for name in safeterm-gateway safeterm-diagnostics safeterm-network safeterm-log-reader safeterm-service-manager safeterm-config-application safeterm-config-database safeterm-config-network; do
  usermod -a -G safeterm-audit "$name"
done
usermod -a -G adm safeterm-log-reader
usermod -a -G safeterm-gateway,safeterm-clients safeterm-broker
usermod -a -G safeterm-gateway,safeterm-clients safeterm
for name in safeterm-config-application safeterm-config-database safeterm-config-network; do
  usermod -a -G safeterm-gateway "$name"
done

for binary in "$bundle"/bin/safeterm-*; do
  install -o root -g root -m 0555 "$binary" "/usr/local/bin/$(basename -- "$binary")"
done
install -o root -g root -m 0555 "$bundle/bin/safeterm-termctl" /usr/local/bin/termctl
usermod --shell /usr/local/bin/safeterm-console safeterm
if ! grep -qxF /usr/local/bin/safeterm-console /etc/shells; then
  echo /usr/local/bin/safeterm-console >>/etc/shells
fi
install -d -o root -g root -m 0755 /etc/safeterm
cp -R "$bundle/etc/safeterm/." /etc/safeterm/
chown -R root:root /etc/safeterm
find /etc/safeterm -type d -exec chmod 0755 {} \;
find /etc/safeterm -type f -exec chmod 0644 {} \;
chown root:safeterm-broker /etc/safeterm/ssh/ssh_host_ed25519_key /etc/safeterm/ssh/users.json
chmod 0640 /etc/safeterm/ssh/ssh_host_ed25519_key /etc/safeterm/ssh/users.json
chown root:safeterm /etc/safeterm/console/users.json
chmod 0640 /etc/safeterm/console/users.json

install -d -o root -g root -m 0755 /etc/safeterm-managed
managed_plan="$work/managed-plan"
"$bundle/bin/safeterm-preflight" --bundle "$bundle" --managed-plan >"$managed_plan"
tab=$(printf '\t')
while IFS="$tab" read -r source target owner; do
  [ -n "$source" ] && [ -n "$target" ] && [ -n "$owner" ] || { echo "invalid managed install plan" >&2; exit 1; }
  install -d -o "$owner" -g "$owner" -m 0700 "$(dirname -- "$target")"
  install -o "$owner" -g "$owner" -m 0600 "$source" "$target"
done <"$managed_plan"
rm -f -- "$managed_plan"
install -d -o root -g root -m 0750 /etc/cat
install -d -o root -g adm -m 0750 /var/log/safeterm-readable
if [ ! -e /var/log/safeterm-readable/service_A.log ]; then
  install -o root -g adm -m 0640 /dev/null /var/log/safeterm-readable/service_A.log
fi
install -d -o root -g root -m 0755 /etc/sudoers.d
install -o root -g root -m 0440 "$sudoers_plan" /etc/sudoers.d/safeterm-service-manager

for unit in "$bundle"/systemd/*.service; do
  install -o root -g root -m 0644 "$unit" "/etc/systemd/system/$(basename -- "$unit")"
done
systemctl daemon-reload
/usr/local/bin/safeterm-preflight --installed

if [ "$start_services" = true ]; then
  systemctl enable safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
  systemctl restart safeterm-audit.service
  systemctl restart safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service
  systemctl restart safeterm-gateway.service
  systemctl restart safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service
  systemctl restart safeterm-ssh-broker.service
  systemctl --quiet is-active safeterm-audit.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service
fi
echo "SafeTerm installation: PASS"
