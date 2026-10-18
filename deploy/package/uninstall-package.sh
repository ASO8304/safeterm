#!/bin/sh
set -eu
[ "$#" -eq 0 ] || { echo "usage: safeterm-uninstall" >&2; exit 2; }
[ "$(id -u)" -eq 0 ] || { echo "uninstall requires root" >&2; exit 1; }
exec 9>/run/lock/safeterm-lifecycle.lock
flock -n 9 || { echo "another SafeTerm lifecycle operation is running" >&2; exit 1; }
[ ! -e /var/lib/safeterm-installer/catalog/pending ] || { echo "interrupted catalog transaction: run sudo termctl apply --recover first" >&2; exit 1; }
# Stop dependents first. Preserve accounts and state so reinstall needs no
# password enrollment and does not destroy historical audit evidence.
for name in ssh-broker config-manager-application config-manager-database config-manager-network gateway runner-service-manager runner-log-reader runner-network-observer runner-diagnostics config-writer identity audit; do
  unit="safeterm-$name.service"
  if systemctl cat "$unit" >/dev/null 2>&1; then
    systemctl stop "$unit"
    systemctl disable "$unit"
  fi
done
for name in audit config-manager config-writer console gateway runner ssh-broker preflight password-hash termctl identity; do
  rm -f -- "/usr/local/bin/safeterm-$name"
done
for name in ssh-broker config-manager-application config-manager-database config-manager-network gateway runner-service-manager runner-log-reader runner-network-observer runner-diagnostics config-writer identity audit; do
  rm -f -- "/etc/systemd/system/safeterm-$name.service"
done
rm -f -- /usr/local/bin/termctl /etc/sudoers.d/safeterm-service-manager
rm -f -- /usr/share/bash-completion/completions/termctl
rm -rf -- /etc/systemd/system/safeterm-config-writer.service.d
systemctl daemon-reload
echo "SafeTerm uninstalled. Configuration, accounts, audit data, signing keys and rollback snapshots retained."
