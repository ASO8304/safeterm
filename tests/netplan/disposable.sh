#!/bin/bash
# Provisions a separate network namespace and Ubuntu guest. Never applies Netplan on the host.
set -euo pipefail
[[ $EUID == 0 && ${SAFETERM_DISPOSABLE_HOST:-} == 1 ]] || { echo 'requires disposable CI host'; exit 2; }
binary=$(realpath "$1")
work=$(mktemp -d)
machine=safeterm-netplan-ci
cleanup() {
  machinectl poweroff "$machine" >/dev/null 2>&1 || true
  if [[ -n ${container_pid:-} ]]; then
    for _ in {1..30}; do kill -0 "$container_pid" 2>/dev/null || break; sleep 1; done
    kill "$container_pid" 2>/dev/null || true
    wait "$container_pid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT
debootstrap --variant=minbase --include=systemd-sysv,dbus,netplan.io,iproute2,systemd-resolved noble "$work/root" http://archive.ubuntu.com/ubuntu
install -d -m 0755 "$work/root/etc/netplan"
systemctl --root="$work/root" enable systemd-networkd.service systemd-resolved.service
install -m 0755 "$binary" "$work/root/opt/network.test"
systemd-nspawn --quiet --directory="$work/root" --machine="$machine" --boot --network-veth >"$work/guest.log" 2>&1 &
container_pid=$!
ready=false
for _ in {1..90}; do
  if machinectl show "$machine" -p Leader --value >/dev/null 2>&1 && systemd-run --machine="$machine" --quiet --wait /usr/bin/systemctl is-active systemd-networkd >/dev/null 2>&1; then ready=true; break; fi
  kill -0 "$container_pid" 2>/dev/null || { cat "$work/guest.log"; exit 1; }
  sleep 1
done
$ready || { cat "$work/guest.log"; echo 'guest did not boot'; exit 1; }
# Match the production manager's main filesystem/capability restrictions.
systemd-run --machine="$machine" --wait --pipe --service-type=exec \
  --property=NoNewPrivileges=yes --property=ProtectSystem=strict \
  --property='ReadWritePaths=/run /etc/netplan /var/lib' \
  --property=PrivateTmp=yes --property=PrivateDevices=yes \
  --property='CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_DAC_OVERRIDE CAP_DAC_READ_SEARCH CAP_CHOWN CAP_FOWNER' \
  --property='RestrictAddressFamilies=AF_UNIX AF_NETLINK AF_INET AF_INET6' \
  --setenv=SAFETERM_NETPLAN_DISPOSABLE=1 \
  /opt/network.test -test.run '^TestDisposableNetplanTrial$' -test.v -test.timeout=180s
