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
termctl user add console --id ci-admin --roles viewer,netops,log-viewer,service-viewer,service-operator,config-viewer,config-operator --password-hash-file "$work/password.hash"
# The legacy demo root must not be manufactured on a fresh installation.
[[ ! -e /etc/safeterm-managed ]]
[[ -f /etc/safeterm/network/policy.json && -f /etc/safeterm/docs/network.md ]]
[[ -f /etc/safeterm/access/ssh/users.json && -f /etc/safeterm/access/console/users.json ]]
[[ -d /etc/safeterm/commands/operations.d && ! -e /etc/safeterm/network.json ]]
wait_ready() {
  for attempt in {1..30}; do
    ready=true
    for socket in /run/safeterm-gateway/gateway.sock /run/safeterm-log-reader/runner.sock /run/safeterm-config-writer/writer.sock \
      /run/safeterm-config-{application,database,network}/manager.sock; do
      [[ -S $socket ]] || ready=false
    done
    if $ready && systemctl --quiet is-active safeterm-runner-log-reader.service; then return; fi
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
# Logical field aliases share one root-owned file and use the real writer.
printf '\357\273\277; logical comment\n[Tunnel]\nTCPMSS = 1360 ; keep\nPATIP = 192.0.2.1 ; retain IP comment\n[Cat]\nPort_2 = 2420\nEnabled = true\n' >/etc/safeterm-ci/logical.ini
chmod 0644 /etc/safeterm-ci/logical.ini
cat >/etc/safeterm/resources.d/logical-options.json <<'JSON'
{"id":"network-settings","description":"Logical settings","service":{"unit":"safeterm-audit.service","actions":{"restart":{"roles":["service-operator"]}}},"config":{"path":"/etc/safeterm-ci/logical.ini","format":"ini","permissions":{"read":["config-viewer","config-operator"],"edit":["config-operator"]},"contexts":[{"path":["tunnel"],"fields":[{"name":"Value_1","section":"Tunnel","key":"TCPMSS","type":"integer","min":1,"max":65535},{"name":"Value_2","section":"Cat","key":"Port_2","type":"port"},{"name":"pat-ips","section":"Tunnel","key":"PATIP","type":"list","item_type":"ipv4","separator":",","required":true,"min_items":1,"max_items":4}]},{"path":["application","cat"],"fields":[{"name":"enabled","section":"Cat","key":"Enabled","type":"boolean"}]}]}}
JSON
termctl check
termctl apply
wait_ready
audit_pid_before=$(systemctl show -p MainPID --value safeterm-audit.service)
python3 "$repo/tests/systemd/console-smoke.py" logical
[[ $(systemctl show -p MainPID --value safeterm-audit.service) == "$audit_pid_before" ]]
printf '\357\273\277; logical comment\n[Tunnel]\nTCPMSS = 1400 ; keep\nPATIP = 192.168.222.10,10.20.30.40 ; retain IP comment\n[Cat]\nPort_2 = 443\nEnabled = false\n' >"$work/expected-logical.ini"
cmp "$work/expected-logical.ini" /etc/safeterm-ci/logical.ini
[[ $(stat -c '%U:%G %a' /etc/safeterm-ci/logical.ini) == 'root:root 644' ]]
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
python3 "$repo/tests/systemd/console-smoke.py" logs
# Removing the last log leaves a healthy, deny-all idle Runner, not a crashed
# service. Exercise both empty -> configured and configured -> empty transitions.
mv /etc/safeterm/resources.d/fresh-log.json /etc/safeterm/resources.d/fresh-log.json.disabled
termctl apply
wait_ready
termctl service status all
bash "$repo/tests/systemd/service-groups.sh" "$repo"
printf 'Systemd fresh installation, Console discovery, completion and INI save: PASS\n'

# Provision independent process and service-manager profiles from declarative sources.
cat >/etc/safeterm/access/roles.d/site-viewer.json <<'JSON'
{"name":"site-viewer","description":"Site read-only role"}
JSON
cat >/etc/safeterm/commands/profiles.d/site-demo.json <<'JSON'
{"name":"site-demo","base":"diagnostics","memory_limit_mib":96,"process_limit":16,"timeout_seconds":10,"max_sessions":2}
JSON
cat >/etc/safeterm/commands/profiles.d/site-services.json <<'JSON'
{"name":"site-services","base":"service-manager"}
JSON
cat >/etc/safeterm/commands/operations.d/site-uname.json <<'JSON'
{"operation":{"name":"site_uname","description":"Isolated site system information","runner_profile":"site-demo","executor_kind":"process","executable_ref":"uname","args_template":["-s"],"parameters":[]},"roles":["viewer","site-viewer"]}
JSON
cat >/etc/safeterm/commands/operations.d/site-status.json <<'JSON'
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
# Simulate a v0.27 flat administrator tree. Updating must preserve credentials,
# the host key, explicit network policy, custom roles and Runner definitions.
cp /etc/safeterm/network/policy.json "$work/policy-before.json"
sha256sum /etc/safeterm/access/ssh/users.json /etc/safeterm/access/ssh/ssh_host_ed25519_key \
  /etc/safeterm/access/console/users.json >"$work/credentials-before.sha256"
for pair in access/ssh:ssh access/console:console access/roles.d:roles.d access/policies.d:policies.d \
  commands/operations.d:operations.d commands/executables.d:executables.d commands/profiles.d:profiles.d \
  network/policy.json:network.json docs/network.md:NETWORK.md docs/network.txt:NETWORK.txt \
  docs/network-diagnostics.md:NETWORK-DIAGNOSTICS.md docs/network-diagnostics.txt:NETWORK-DIAGNOSTICS.txt; do
  mv "/etc/safeterm/${pair%%:*}" "/etc/safeterm/${pair#*:}"
done
install -d -m 0755 /etc/safeterm-managed/{application,database,network}
termctl update "$package"
sha256sum -c "$work/credentials-before.sha256"
cmp "$work/policy-before.json" /etc/safeterm/network/policy.json
[[ ! -e /etc/safeterm/network.json && ! -e /etc/safeterm/roles.d && ! -e /etc/safeterm-managed ]]
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" site
# An unchanged generation can also update using its verified prior signature.
mv /root/.config/safeterm/catalog-ed25519.key "$work/offline.key"
termctl update "$package"
wait_ready
python3 "$repo/tests/systemd/console-smoke.py" site
mv "$work/offline.key" /root/.config/safeterm/catalog-ed25519.key
printf 'Declarative roles, profiles, operations, scoped sudo, rollback and signed offline update: PASS\n'
