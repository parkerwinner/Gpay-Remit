#!/usr/bin/env python3
"""Validate and pin the Soroban test regression corpus and source revision."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
CONTRACTS = ROOT / "contracts"
SNAPSHOTS = CONTRACTS / "test_snapshots"
MANIFEST = SNAPSHOTS / ".manifest.json"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def current_manifest() -> dict:
    files = {}
    for path in sorted(SNAPSHOTS.rglob("*.json")):
        if path == MANIFEST:
            continue
        relative = path.relative_to(CONTRACTS).as_posix()
        json.loads(path.read_text(encoding="utf-8"))
        files[relative] = digest(path)
    sources = sorted(list((CONTRACTS / "src").rglob("*.rs")) + list((CONTRACTS / "tests").rglob("*.rs")))
    source_hash = hashlib.sha256()
    for path in sources:
        source_hash.update(path.relative_to(CONTRACTS).as_posix().encode())
        source_hash.update(path.read_bytes())
    return {"source_sha256": source_hash.hexdigest(), "regression_cases": files}


if os.environ.get("UPDATE_SNAPSHOTS") == "1":
    completed = subprocess.run(
        ["cargo", "test", "--locked", "--all-features", "--no-fail-fast"],
        cwd=CONTRACTS,
        check=False,
    )
    if completed.returncode:
        sys.exit(completed.returncode)
    MANIFEST.write_text(json.dumps(current_manifest(), indent=2) + "\n", encoding="utf-8")
    print(f"Updated regression corpus manifest: {MANIFEST.relative_to(ROOT)}")
    sys.exit(0)

if not MANIFEST.exists():
    sys.exit("Snapshot manifest is missing; run UPDATE_SNAPSHOTS=1 python scripts/check_contract_snapshots.py")
expected = json.loads(MANIFEST.read_text(encoding="utf-8"))
actual = current_manifest()
if actual != expected:
    sys.exit("Contract source or regression snapshots changed. Run UPDATE_SNAPSHOTS=1 python scripts/check_contract_snapshots.py and review the result.")
print(f"Validated {len(actual['regression_cases'])} committed regression snapshots.")
