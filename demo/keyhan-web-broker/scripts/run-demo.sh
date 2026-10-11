#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "$ROOT/.venv/bin/gunicorn" -c "$ROOT/gunicorn.conf.py" keyhan.web_broker.app:app
