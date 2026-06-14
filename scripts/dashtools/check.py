"""Validate a dashboard JSON against its captured expr-set baseline."""
import json
import sys
from pathlib import Path

from lib import load, expr_set

BASELINE_DIR = Path("scripts/dashtools/baseline")


def main(path, capture=False):
    dash = load(path)
    exprs = expr_set(dash)
    base_file = BASELINE_DIR / (Path(path).stem + ".exprs.json")
    if capture:
        BASELINE_DIR.mkdir(parents=True, exist_ok=True)
        with open(base_file, "w") as f:
            json.dump(exprs, f, indent=2)
        print(f"CAPTURED {len(exprs)} exprs -> {base_file}")
        return 0
    with open(base_file) as f:
        baseline = json.load(f)
    if exprs != baseline:
        added = set(exprs) - set(baseline)
        removed = set(baseline) - set(exprs)
        print(f"FAIL {path}: expr-set changed. added={added} removed={removed}")
        return 1
    print(f"OK {path}: {len(exprs)} exprs unchanged")
    return 0


if __name__ == "__main__":
    capture = "--capture" in sys.argv
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    sys.exit(main(args[0], capture=capture))
