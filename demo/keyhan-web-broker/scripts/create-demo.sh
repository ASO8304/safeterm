#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PYTHON_BIN=python3
VENV="$ROOT/.venv"
cd "$ROOT"
"$PYTHON_BIN" -m venv "$VENV"
"$VENV/bin/python" -m pip install --upgrade pip
"$VENV/bin/pip" install -r "$ROOT/requirements.txt"
"$VENV/bin/python" -m pytest -q "$ROOT/tests"
echo "Keyhan Web Broker demo: PASS"
