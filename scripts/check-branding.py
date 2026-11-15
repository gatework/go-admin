#!/usr/bin/env python3
"""Check active backend sources for retired product spellings."""
from pathlib import Path
import re
import sys

root = Path(__file__).resolve().parent.parent
pattern = re.compile(r"go[-_ ]?admin(?!-ui)", re.IGNORECASE)
failures = []
for path in root.rglob("*"):
    relative = path.relative_to(root)
    if not path.is_file() or any(part in {".git", "vendor", "temp"} for part in relative.parts):
        continue
    if relative.as_posix().startswith("cmd/migrate/migration/version/"):
        continue  # Frozen migration history; imports are checked by the compiler.
    if path.name.startswith("LICENSE") or path == Path(__file__).resolve():
        continue
    try:
        lines = path.read_text().splitlines()
    except (UnicodeError, OSError):
        continue
    for number, line in enumerate(lines, 1):
        if line.startswith("Upstream attribution:"):
            continue
        if pattern.search(line):
            failures.append(f"{relative}:{number}: retired product spelling")
for failure in failures:
    print(failure)
sys.exit(bool(failures))
