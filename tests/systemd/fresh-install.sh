#!/bin/bash
# Destructive integration test: run ONLY on a fresh disposable CI VM.
set -euo pipefail
[[ ${SAFETERM_DISPOSABLE_HOST:-} == 1 && $EUID == 0 ]] || {
  echo 'requires root and SAFETERM_DISPOSABLE_HOST=1 on a disposable VM' >&2; exit 2;
}
[[ ! -e /etc/safeterm && ! -e /var/lib/safeterm/generated ]] || {
  echo 'refusing an existing SafeTerm installation' >&2; exit 2;
}
package=$(realpath "$1")
repo=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
on_failure() {
  systemctl --no-pager -l status 'safeterm-*' || true
  journalctl --no-pager -n 250 -u 'safeterm-*' || true
}
trap on_failure ERR
printf 'SmokeTest42\n' | "$package/payload/bin/safeterm-password-hash" --stdin --output "$work/password.hash"
"$package/install.sh" --local-signing --non-interactive --install-dependencies \
  --ssh-user ci-admin --ssh-password-hash-file "$work/password.hash"
termctl user add console --id ci-admin --roles viewer,netops,config-viewer,config-operator --password-hash-file "$work/password.hash"
# These demo directories must not be required, or manufactured as a workaround.
for profile in application database network; do
  [[ ! -e /etc/safeterm-managed/$profile ]]
done
wait_ready() {
  for attempt in {1..30}; do
    ready=true
    for socket in /run/safeterm-gateway/gateway.sock /run/safeterm-config-writer/writer.sock \
      /run/safeterm-config-{application,database,network}/manager.sock; do
      [[ -S $socket ]] || ready=false
    done
    if $ready; then return; fi
    sleep 1
  done
  echo 'required SafeTerm sockets never became ready' >&2; return 1
}
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" empty
install -d -m 0755 /etc/safeterm-ci
printf '; untouched comment\n[Server]\nPort = 8080 ; keep comment\n' >/etc/safeterm-ci/options.ini
chmod 0644 /etc/safeterm-ci/options.ini
cat >/etc/safeterm/resources.d/fresh-options.json <<'JSON'
{"id":"fresh-options","description":"CI configuration","config":{"path":"/etc/safeterm-ci/options.ini","format":"ini","permissions":{"read":["config-viewer","config-operator"],"edit":["config-operator"]},"fields":[{"section":"Server","key":"Port","type":"integer","min":1,"max":65535}]}}
JSON
termctl check
termctl apply
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" configured
printf '; untouched comment\n[Server]\nPort = 9090 ; keep comment\n' >"$work/expected.ini"
cmp "$work/expected.ini" /etc/safeterm-ci/options.ini
[[ $(stat -c '%U:%G %a' /etc/safeterm-ci/options.ini) == 'root:root 644' ]]
termctl service status all
printf 'Systemd fresh installation, Console discovery, completion and INI save: PASS\n'
