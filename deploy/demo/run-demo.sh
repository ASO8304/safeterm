#!/bin/sh
set -eu

[ "$(id -u)" -eq 0 ] || { echo "the full systemd demo requires root" >&2; exit 1; }
for tool in go ssh ssh-keygen ssh-keyscan systemctl tcpdump; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 1; }
done

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/safeterm-demo.XXXXXX")
cleanup() { rm -rf -- "$work_dir"; }
trap cleanup EXIT HUP INT TERM

mkdir -p "$work_dir/bin" "$work_dir/secrets"
(cd "$repo_dir" && go build -o "$work_dir/bin/catalog-prepare" ./cmd/catalog-prepare)
(cd "$repo_dir" && go build -o "$work_dir/bin/catalog-sign" ./cmd/catalog-sign)
"$work_dir/bin/catalog-prepare" --source "$repo_dir/configs" --output "$work_dir/config"
"$work_dir/bin/catalog-sign" -generate-key -private-key "$work_dir/secrets/catalog.key" -public-key "$work_dir/config/trust/catalog-ed25519.pub"
"$work_dir/bin/catalog-sign" -config "$work_dir/config" -private-key "$work_dir/secrets/catalog.key" -signature "$work_dir/config/catalog.sig"

mkdir -p "$work_dir/config/ssh" "$work_dir/config/config-manager"
ssh-keygen -q -t ed25519 -N '' -f "$work_dir/config/ssh/ssh_host_ed25519_key"
ssh-keygen -q -t ed25519 -N '' -f "$work_dir/secrets/alice_ed25519"
alice_key=$(cat "$work_dir/secrets/alice_ed25519.pub")
cat >"$work_dir/config/ssh/users.json" <<EOF
{"users":[{"user_id":"alice","roles":["viewer","netops","config-viewer","config-operator","config-approver"],"authorized_keys":["$alice_key"]}]}
EOF
cp "$repo_dir/deploy/config-manager/resources.example.json" "$work_dir/config/config-manager/resources.json"
chmod 0600 "$work_dir/config/ssh/ssh_host_ed25519_key" "$work_dir/config/ssh/users.json"
chmod 0644 "$work_dir/config/catalog.sig" "$work_dir/config/trust/catalog-ed25519.pub" "$work_dir/config/config-manager/resources.json"

"$repo_dir/deploy/scripts/build-bundle.sh" --config "$work_dir/config" --managed "$repo_dir/deploy/demo/managed" --output "$work_dir/bundle"
"$repo_dir/deploy/scripts/install.sh" --bundle "$work_dir/bundle"

known_hosts="$work_dir/known_hosts"
attempt=0
while [ "$attempt" -lt 30 ]; do
  if ssh-keyscan -p 2222 127.0.0.1 >"$known_hosts" 2>/dev/null; then break; fi
  attempt=$((attempt + 1))
  sleep 1
done
[ -s "$known_hosts" ] || { echo "SSH Broker did not become ready" >&2; exit 1; }

output="$work_dir/demo-output.txt"
printf 'whoami\rlist\rrun system_info\rconfig list\rexit\r' | \
  ssh -tt -p 2222 -i "$work_dir/secrets/alice_ed25519" -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=yes -o UserKnownHostsFile="$known_hosts" alice@127.0.0.1 >"$output"
grep -q 'Authenticated as alice' "$output"
grep -q 'system_info' "$output"
grep -q 'result: success' "$output"
grep -q 'cats-options' "$output"
/usr/local/bin/safeterm-preflight --installed
cat "$output"
echo "SafeTerm end-to-end demo: PASS"
