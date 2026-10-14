#!/bin/sh
set -eu

transaction_root=/var/backups/safeterm-installer
units='safeterm-audit.service safeterm-identity.service safeterm-runner-diagnostics.service safeterm-runner-network-observer.service safeterm-runner-log-reader.service safeterm-runner-service-manager.service safeterm-gateway.service safeterm-config-manager-application.service safeterm-config-manager-database.service safeterm-config-manager-network.service safeterm-ssh-broker.service'

usage() {
  echo "usage: $0 [latest|TRANSACTION_ID]" >&2
  exit 2
}

[ "$#" -le 1 ] || usage
[ "$(id -u)" -eq 0 ] || { echo "rollback requires root" >&2; exit 1; }
exec 9>/run/lock/safeterm-lifecycle.lock
flock -n 9 || { echo "another SafeTerm lifecycle operation is running" >&2; exit 1; }
selection=${1:-latest}
case "$selection" in ''|*[!A-Za-z0-9._-]*) usage ;; esac
if [ "$selection" = latest ]; then
  [ -f "$transaction_root/latest" ] || { echo "no SafeTerm installation rollback is available" >&2; exit 1; }
  transaction_id=$(cat "$transaction_root/latest")
else
  transaction_id=$selection
fi
case "$transaction_id" in ''|*[!A-Za-z0-9._-]*) echo "invalid rollback transaction" >&2; exit 1 ;; esac
transaction="$transaction_root/$transaction_id"
[ -d "$transaction" ] || { echo "rollback transaction not found: $transaction_id" >&2; exit 1; }

(cd "$transaction" && sha256sum -c SNAPSHOT-SHA256SUMS)

# Validate every removal path before stopping services or deleting files.
while IFS= read -r path; do
  case "$path" in
    /usr/share/bash-completion/completions/termctl|/usr/local/bin/termctl|/usr/local/sbin/safeterm-uninstall) ;;
    /etc/safeterm|/etc/safeterm-managed|/etc/cat|/etc/sudoers.d/safeterm-service-manager|/home/safeterm|/var/lib/safeterm|/var/lib/safeterm-installer|/var/log/safeterm-readable|/var/mail/safeterm|/var/spool/mail/safeterm|/usr/local/sbin/safeterm-install-rollback|/root/?*|/usr/local/bin/safeterm-*|/etc/systemd/system/safeterm-*.service) ;;
    *) echo "unsafe rollback path: $path" >&2; exit 1 ;;
  esac
done <"$transaction/replace-paths"

for unit in $units; do
  systemctl stop "$unit" >/dev/null 2>&1 || true
done
for unit in $units; do
  systemctl disable "$unit" >/dev/null 2>&1 || true
done

while IFS= read -r path; do
  [ -n "$path" ] || continue
  case "$path" in
    /usr/share/bash-completion/completions/termctl|/usr/local/bin/termctl|/usr/local/sbin/safeterm-uninstall) ;;
    /etc/safeterm|/etc/safeterm-managed|/etc/cat|/etc/sudoers.d/safeterm-service-manager|/home/safeterm|/var/lib/safeterm|/var/lib/safeterm-installer|/var/log/safeterm-readable|/var/mail/safeterm|/var/spool/mail/safeterm|/usr/local/sbin/safeterm-install-rollback|/root/?*|/usr/local/bin/safeterm-*|/etc/systemd/system/safeterm-*.service) ;;
    *) echo "unsafe rollback path: $path" >&2; exit 1 ;;
  esac
  rm -rf -- "$path"
done <"$transaction/replace-paths"

# Account support files are normally present on Ubuntu, but remove any that the
# installer created when the snapshot explicitly records their prior absence.
while IFS= read -r path; do
  case "$path" in
    /etc/passwd|/etc/shadow|/etc/group|/etc/gshadow|/etc/subuid|/etc/subgid|/etc/shells|/var/log/lastlog|/var/log/faillog) rm -f -- "$path" ;;
  esac
done <"$transaction/absent-paths"

tar -C / -xpf "$transaction/snapshot.tar"
systemctl daemon-reload

while IFS=' ' read -r unit enabled active; do
  [ -n "$unit" ] || continue
  case "$enabled" in
    enabled|linked) systemctl enable "$unit" >/dev/null ;;
    enabled-runtime|linked-runtime) systemctl enable --runtime "$unit" >/dev/null ;;
    masked) systemctl mask "$unit" >/dev/null ;;
    masked-runtime) systemctl mask --runtime "$unit" >/dev/null ;;
  esac
done <"$transaction/service-state"
while IFS=' ' read -r unit enabled active; do
  [ -n "$unit" ] || continue
  [ "$active" = active ] || continue
  systemctl start "$unit"
done <"$transaction/service-state"

if [ -x /usr/local/bin/safeterm-preflight ] && [ -d /etc/safeterm ]; then
  /usr/local/bin/safeterm-preflight --installed
fi

rm -rf -- "$transaction"
remaining=$(find "$transaction_root" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | LC_ALL=C sort | tail -n 1)
if [ -n "$remaining" ]; then
  printf '%s\n' "$remaining" >"$transaction_root/latest"
else
  rm -f -- "$transaction_root/latest"
  rmdir "$transaction_root" 2>/dev/null || true
fi
echo "SafeTerm installation rollback: PASS ($transaction_id)"
