#!/bin/sh
set -eu

usage() {
  echo "usage: $0 --config DIR --managed DIR --output DIR" >&2
  exit 2
}

config_dir=
managed_dir=
output_dir=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --config) [ "$#" -ge 2 ] || usage; config_dir=$2; shift 2 ;;
    --managed) [ "$#" -ge 2 ] || usage; managed_dir=$2; shift 2 ;;
    --output) [ "$#" -ge 2 ] || usage; output_dir=$2; shift 2 ;;
    *) usage ;;
  esac
done
[ -n "$config_dir" ] && [ -d "$config_dir" ] && [ -n "$managed_dir" ] && [ -d "$managed_dir" ] && [ -n "$output_dir" ] || usage
[ ! -e "$output_dir" ] || { echo "output already exists: $output_dir" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 1; }

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
parent_dir=$(dirname -- "$output_dir")
mkdir -p "$parent_dir"
staging=$(mktemp -d "$parent_dir/.safeterm-bundle.XXXXXX")
cleanup() { rm -rf -- "$staging"; }
trap cleanup EXIT HUP INT TERM

if find "$config_dir" "$managed_dir" -type l -print -quit | grep -q .; then
  echo "configuration and managed inputs must not contain symlinks" >&2
  exit 1
fi

mkdir -p "$staging/bin" "$staging/etc" "$staging/systemd"
for entry in audit config-manager config-writer console gateway runner ssh-broker preflight termctl identity; do
  (cd "$repo_dir" && go build -trimpath -o "$staging/bin/safeterm-$entry" "./cmd/$entry")
  chmod 0555 "$staging/bin/safeterm-$entry"
done
cp -R "$config_dir" "$staging/etc/safeterm"
cp "$repo_dir/configs/README.md" "$repo_dir/configs/README.txt" "$staging/etc/safeterm/"
mkdir -p "$staging/etc/safeterm/resources.d"
cp "$repo_dir/configs/resources.d/TEMPLATE.json" "$repo_dir/configs/resources.d/README.md" "$repo_dir/configs/resources.d/README.txt" "$staging/etc/safeterm/resources.d/"
cp -R "$managed_dir" "$staging/managed"
cp "$repo_dir"/deploy/systemd/*.service "$staging/systemd/"
chmod 0644 "$staging"/systemd/*.service
chmod 0600 "$staging"/managed/*/*

(cd "$staging" && find bin etc managed systemd -type f -print | LC_ALL=C sort | xargs sha256sum > SHA256SUMS)
chmod 0444 "$staging/SHA256SUMS"
"$staging/bin/safeterm-preflight" --bundle "$staging"
mv -- "$staging" "$output_dir"
trap - EXIT HUP INT TERM
echo "SafeTerm bundle created: $output_dir"
