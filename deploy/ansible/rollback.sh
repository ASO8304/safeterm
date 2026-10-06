#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
inventory=${SAFETERM_INVENTORY:-$script_dir/inventory.yml}
transaction=latest

usage() {
  echo "usage: $0 [--inventory FILE] [--transaction latest|ID] [--limit HOST_PATTERN]" >&2
  exit 2
}

limit=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --inventory) [ "$#" -ge 2 ] || usage; inventory=$2; shift 2 ;;
    --transaction) [ "$#" -ge 2 ] || usage; transaction=$2; shift 2 ;;
    --limit) [ "$#" -ge 2 ] || usage; limit=$2; shift 2 ;;
    --help|-h) usage ;;
    *) usage ;;
  esac
done
case "$transaction" in ''|*[!A-Za-z0-9._-]*) usage ;; esac
command -v ansible-playbook >/dev/null 2>&1 || { echo "ansible-playbook is required" >&2; exit 1; }

set -- ansible-playbook -i "$inventory" "$script_dir/rollback.yml" -e "safeterm_rollback_transaction=$transaction"
[ -z "$limit" ] || set -- "$@" --limit "$limit"
ANSIBLE_CONFIG="$script_dir/ansible.cfg" exec "$@"
