#!/bin/bash
# Called only by the disposable-host fresh installation test.
set -euo pipefail
[[ ${SAFETERM_DISPOSABLE_HOST:-} == 1 && $EUID == 0 ]] || exit 2
repo=$1
cat >/etc/safeterm-ci/service-event.py <<'PY'
import sys
with open('/etc/safeterm-ci/service-events', 'a') as events:
    events.write(' '.join(sys.argv[1:]) + '\n')
PY
for name in db api web; do
  cat >"/etc/systemd/system/ci-chain-$name.service" <<EOF
[Unit]
Description=Disposable service group fixture
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/python3 /etc/safeterm-ci/service-event.py start $name
ExecStop=/usr/bin/python3 /etc/safeterm-ci/service-event.py stop $name
EOF
done
systemctl daemon-reload
cat >/etc/safeterm/resources.d/application-stack.json <<'JSON'
{"id":"application-stack","description":"Application stack","service":{"units":["ci-chain-db.service","ci-chain-api.service","ci-chain-web.service"],"timeout_seconds":120,"actions":{"start":{"roles":["service-operator"]},"stop":{"roles":["service-operator"]},"restart":{"roles":["service-operator"]},"status":{"roles":["service-viewer"]}}}}
JSON
termctl check
termctl apply
for attempt in {1..30}; do
  [[ -S /run/safeterm-gateway/gateway.sock && -S /run/safeterm-service-manager/runner.sock ]] && break
  sleep 1
done
python3 "$repo/tests/systemd/console-smoke.py" services
python3 - <<'PY'
import json
from pathlib import Path
expected = ['start db', 'start api', 'start web',
            'stop web', 'stop api', 'stop db',
            'start db', 'start api', 'start web',
            'stop web', 'stop api', 'stop db']
actual = Path('/etc/safeterm-ci/service-events').read_text().splitlines()
assert actual == expected, actual
events = [json.loads(line)['event'] for line in
          Path('/var/lib/safeterm/audit/audit.jsonl').read_text().splitlines()]
steps = [e for e in events if e.get('service_unit', '').startswith('ci-chain-')]
assert len(steps) == 30, len(steps)  # 15 steps, before and after durable events
assert all(e.get('user_id') == 'ci-admin' and e.get('request_id') for e in steps)
assert all(e['outcome'] in ('service-step-started', 'service-step-completed') for e in steps)
PY
printf 'Ordered service groups, alias output, completion and durable per-step audit: PASS\n'
