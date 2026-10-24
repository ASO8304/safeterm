"""Expose bounded test-only failure context in a GitHub Actions annotation."""
import sys
from pathlib import Path

for name in sys.argv[1:]:
    path = Path(name)
    if not path.is_file():
        continue
    lines = path.read_text(errors="replace").splitlines()
    selected = set()
    for index, line in enumerate(lines):
        if any(marker in line for marker in ("--- FAIL:", "panic:", "fatal error:", "FAIL\t", "FAIL ", "Error:", "make: ***")):
            selected.update(range(max(0, index - 3), min(len(lines), index + 28)))
    if not selected:
        selected.update(range(max(0, len(lines) - 80), len(lines)))
    message = "\n".join(lines[index] for index in sorted(selected))[-50000:]
    message = message.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
    print(f"::error title=Test failure diagnostics::{message}")
