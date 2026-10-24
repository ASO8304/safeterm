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
printf '\357\273\277[Server]\n\n; untouched comment\n\nPort = 8080 ; keep comment\n' >/etc/safeterm-ci/options.ini
chmod 0644 /etc/safeterm-ci/options.ini
cat >/etc/safeterm/resources.d/fresh-options.json <<'JSON'
{"id":"fresh-options","description":"CI configuration","config":{"path":"/etc/safeterm-ci/options.ini","format":"ini","permissions":{"read":["config-viewer","config-operator"],"edit":["config-operator"]},"fields":[{"section":"Server","key":"Port","type":"integer","min":1,"max":65535}]}}
JSON
termctl check
termctl apply
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" configured
printf '\357\273\277[Server]\n\n; untouched comment\n\nPort = 9090 ; keep comment\n' >"$work/expected.ini"
cmp "$work/expected.ini" /etc/safeterm-ci/options.ini
[[ $(stat -c '%U:%G %a' /etc/safeterm-ci/options.ini) == 'root:root 644' ]]
# Exercise the real production access probe before and after a permissions failure.
printf 'CI log fixture\n' >/var/log/safeterm-ci.log
chmod 0644 /var/log/safeterm-ci.log
cat >/etc/safeterm/resources.d/fresh-log.json <<'JSON'
{"id":"fresh-log","description":"CI registered log","logs":[{"id":"main","path":"/var/log/safeterm-ci.log","roles":["log-viewer"]}]}
JSON
termctl check
chmod 0600 /var/log/safeterm-ci.log
if termctl check >"$work/unreadable-log.log" 2>&1; then
  echo 'check accepted a log the reader cannot read' >&2; exit 1
fi
grep -F 'read-access check for log "main"' "$work/unreadable-log.log"
if grep -F 'unsupported catalog administration' "$work/unreadable-log.log"; then exit 1; fi
chmod 0644 /var/log/safeterm-ci.log
termctl apply
wait_ready
termctl service status all
printf 'Systemd fresh installation, Console discovery, completion and INI save: PASS\n'

# Provision independent process and service-manager profiles from declarative sources.
cat >/etc/safeterm/roles.d/site-viewer.json <<'JSON'
{"name":"site-viewer","description":"Site read-only role"}
JSON
cat >/etc/safeterm/profiles.d/site-demo.json <<'JSON'
{"name":"site-demo","base":"diagnostics","memory_limit_mib":96,"process_limit":16,"timeout_seconds":10,"max_sessions":2}
JSON
cat >/etc/safeterm/profiles.d/site-services.json <<'JSON'
{"name":"site-services","base":"service-manager"}
JSON
cat >/etc/safeterm/operations.d/site-uname.json <<'JSON'
{"operation":{"name":"site_uname","description":"Isolated site system information","runner_profile":"site-demo","executor_kind":"process","executable_ref":"uname","args_template":["-s"],"parameters":[]},"roles":["viewer","site-viewer"]}
JSON
cat >/etc/safeterm/operations.d/site-status.json <<'JSON'
{"operation":{"name":"site_status","description":"Isolated service status","runner_profile":"site-services","executor_kind":"systemd","systemd":{"unit":"safeterm-gateway.service","action":"status"},"parameters":[]},"roles":["viewer","site-viewer"]}
JSON
termctl check
termctl apply
wait_ready
termctl role describe site-viewer | grep -q site_uname
systemctl --quiet is-active safeterm-runner-site-site-demo.service safeterm-runner-site-site-services.service
[[ $(id -u sft-site-site-demo) != "$(id -u safeterm-diagnostics)" ]]
[[ $(systemctl show safeterm-runner-site-site-demo.service -p MemoryMax --value) == 100663296 ]]
if sudo -n -u sft-site-site-demo head -n 1 /etc/shadow >/dev/null 2>&1; then echo 'site Runner read shadow' >&2; exit 1; fi
python3 "$repo/tests/systemd/console-smoke.py" site
if sudo -n -u sft-site-site-services sudo -n /usr/bin/systemctl --no-pager restart safeterm-ssh-broker.service >/dev/null 2>&1; then echo 'site profile inherited another Runner sudo grant' >&2; exit 1; fi
termctl apply --rollback
[[ ! -e /etc/systemd/system/safeterm-runner-site-site-demo.service ]]
[[ ! -e /run/safeterm-site-site-demo/runner.sock ]]
# Sources remain; applying them again reuses the locked managed UID safely.
termctl apply
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" site
# New-format updates preserve identities that actually use a custom role.
termctl user auth console --id ci-admin --roles viewer,netops,config-viewer,config-operator,site-viewer
# New-format package updates preserve all source directories and re-provision Runner units.
termctl update "$package"
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" site
# An unchanged generation can also update using its verified prior signature.
mv /root/.config/safeterm/catalog-ed25519.key "$work/offline.key"
termctl update "$package"
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" site
mv "$work/offline.key" /root/.config/safeterm/catalog-ed25519.key
printf 'Declarative roles, profiles, operations, scoped sudo, rollback and signed offline update: PASS\n'
