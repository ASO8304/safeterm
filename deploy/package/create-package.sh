#!/bin/sh
set -eu

usage() {
  echo "usage: $0 --output DIR [--arch amd64|arm64]" >&2
  exit 2
}

output=
target_arch=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) [ "$#" -ge 2 ] || usage; output=$2; shift 2 ;;
    --arch) [ "$#" -ge 2 ] || usage; target_arch=$2; shift 2 ;;
    *) usage ;;
  esac
done

[ -n "$output" ] || usage
[ ! -e "$output" ] || { echo "output already exists: $output" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "go is required on the package-building machine" >&2; exit 1; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 1; }
[ "$(go env GOOS)" = linux ] || { echo "packages must be built on Linux" >&2; exit 1; }
[ -n "$target_arch" ] || target_arch=$(go env GOARCH)
case "$target_arch" in amd64|arm64) ;; *) echo "unsupported architecture: $target_arch" >&2; exit 1 ;; esac

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
output=$(realpath -m -- "$output")
parent=$(dirname -- "$output")
mkdir -p "$parent"
staging=$(mktemp -d "$parent/.safeterm-package.XXXXXX")
cleanup() { rm -rf -- "$staging"; }
trap cleanup EXIT HUP INT TERM

mkdir -p "$staging/payload/bin" "$staging/payload/catalog-source" "$staging/payload/managed" \
  "$staging/payload/console" "$staging/payload/systemd" \
  "$staging/payload/templates"

for entry in audit config-manager config-writer console gateway runner ssh-broker preflight termctl catalog-prepare catalog-sign password-hash identity; do
  (cd "$repo_dir" && CGO_ENABLED=0 GOOS=linux GOARCH="$target_arch" \
    go build -trimpath -o "$staging/payload/bin/safeterm-$entry" "./cmd/$entry")
  chmod 0555 "$staging/payload/bin/safeterm-$entry"
done

cp -R "$repo_dir/internal/defaults/catalog/." "$staging/payload/catalog-source/"
cp "$repo_dir/configs/README.md" "$repo_dir/configs/README.txt" "$staging/payload/catalog-source/"
for definitions in resources.d roles.d profiles.d operations.d executables.d policies.d; do
  cp -R "$repo_dir/configs/$definitions" "$staging/payload/catalog-source/"
done
cp -R "$repo_dir/deploy/demo/managed/." "$staging/payload/managed/"
cp "$repo_dir/deploy/runtime/resources.json" "$staging/payload/resources.json"
cp "$repo_dir/deploy/console/users.example.json" "$staging/payload/console/users.json"
cp "$repo_dir/deploy/console/users.template.jsonc" "$staging/payload/templates/console-users.template.jsonc"
cp "$repo_dir/deploy/ssh/users.template.jsonc" "$staging/payload/templates/ssh-users.template.jsonc"
cp "$repo_dir/deploy/systemd/"*.service "$staging/payload/systemd/"
cp "$repo_dir/deploy/scripts/install.sh" "$staging/payload/base-install.sh"
cp "$repo_dir/deploy/package/install-support.sh" "$staging/payload/install-support.sh"
cp "$repo_dir/deploy/package/install-package.sh" "$staging/install.sh"
cp "$repo_dir/deploy/package/rollback-package.sh" "$staging/payload/rollback.sh"
cp "$repo_dir/deploy/package/uninstall-package.sh" "$staging/payload/uninstall.sh"
chmod 0555 "$staging/payload/uninstall.sh"
chmod 0555 "$staging/install.sh" "$staging/payload/base-install.sh" "$staging/payload/rollback.sh"
printf '%s\n' "$target_arch" >"$staging/TARGET_ARCH"

(cd "$staging" && find TARGET_ARCH install.sh payload -type f -print | LC_ALL=C sort | xargs sha256sum >PACKAGE-SHA256SUMS)
chmod 0444 "$staging/PACKAGE-SHA256SUMS"
mv -- "$staging" "$output"
trap - EXIT HUP INT TERM
echo "SafeTerm offline installation package created: $output"
echo "Transfer this directory to the target host and run: sudo ./install.sh"
