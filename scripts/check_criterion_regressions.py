#!/usr/bin/env python3
"""Fail when a Criterion mean estimate is more than 10% slower than main."""

import json
import sys
from pathlib import Path


def estimate(path: Path) -> float:
    with path.open(encoding="utf-8") as results:
        return float(json.load(results)["mean"]["point_estimate"])


def main() -> int:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else "contracts/target/criterion")
    baselines = sorted(root.glob("**/main/estimates.json"))
    if not baselines:
        print(f"No Criterion main baselines found under {root}", file=sys.stderr)
        return 2

    regressions = []
    compared = 0
    for baseline_file in baselines:
        current_file = baseline_file.parent.parent / "new" / "estimates.json"
        if not current_file.exists():
            print(f"Missing current result for {baseline_file.parent.parent}", file=sys.stderr)
            regressions.append(str(baseline_file.parent.parent))
            continue

        baseline = estimate(baseline_file)
        current = estimate(current_file)
        compared += 1
        change = (current / baseline - 1) * 100 if baseline > 0 else 0
        print(f"{baseline_file.parent.parent}: {change:+.2f}% (main={baseline:.2f}, current={current:.2f})")
        if baseline > 0 and current > baseline * 1.10:
            regressions.append(f"{baseline_file.parent.parent} regressed by {change:.2f}%")

    if compared == 0:
        print("No benchmark results were compared", file=sys.stderr)
        return 2
    if regressions:
        print("Benchmark regression threshold exceeded (>10%):", file=sys.stderr)
        print("\n".join(regressions), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
