#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
inventory=${SAFETERM_INVENTORY:-$script_dir/inventory.yml}
variables=${SAFETERM_VARS:-$script_dir/group_vars/all.yml}

usage() {
  cat >&2 <<EOF
usage: $0 [--inventory FILE] [--vars FILE] [--limit HOST_PATTERN] [--check]

Defaults:
  inventory: $inventory
  variables: $variables
EOF
  exit 2
}

limit=
check=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --inventory) [ "$#" -ge 2 ] || usage; inventory=$2; shift 2 ;;
    --vars) [ "$#" -ge 2 ] || usage; variables=$2; shift 2 ;;
    --limit) [ "$#" -ge 2 ] || usage; limit=$2; shift 2 ;;
    --check) check=--check; shift ;;
    --help|-h) usage ;;
    *) usage ;;
  esac
done

command -v ansible-playbook >/dev/null 2>&1 || {
  echo "ansible-playbook is required on the controller" >&2
  exit 1
}
[ -f "$inventory" ] || { echo "inventory not found: $inventory" >&2; exit 1; }
[ -f "$variables" ] || { echo "variables file not found: $variables" >&2; exit 1; }

set -- ansible-playbook -i "$inventory" "$script_dir/install.yml" -e "@$variables"
[ -z "$limit" ] || set -- "$@" --limit "$limit"
[ -z "$check" ] || set -- "$@" "$check"
ANSIBLE_CONFIG="$script_dir/ansible.cfg" exec "$@"
