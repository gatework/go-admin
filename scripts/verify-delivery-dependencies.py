#!/usr/bin/env python3
"""Verify versioned network modules without a workspace or local replacements."""
import json
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
env = dict(os.environ, GOWORK="off")
output = subprocess.check_output(
    ["go", "list", "-mod=readonly", "-m", "-json", "all"],
    cwd=root, env=env, text=True,
)
decoder = json.JSONDecoder()
while output.strip():
    module, end = decoder.raw_decode(output.lstrip())
    output = output.lstrip()[end:]
    path = module["Path"]
    replacement = module.get("Replace")
    if replacement and not replacement.get("Version"):
        raise SystemExit(f"Local module replacement is not allowed: {path}")
    if path.startswith("github.com/gatework/"):
        if replacement or not all(module.get(key) for key in ("Version", "Sum", "GoModSum")):
            raise SystemExit(f"Missing pinned, downloaded GitHub module: {path}")
        print(f"{path} {module['Version']}: checksums recorded", flush=True)

subprocess.run(["go", "mod", "verify"], cwd=root, env=env, check=True)
