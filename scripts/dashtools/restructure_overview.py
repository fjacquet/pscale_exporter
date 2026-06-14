"""Reorder Overview rows into RED/USE order and collapse the Per-Node row."""
from lib import OVERVIEW, load, dump, group_rows, emit_rows

# CURRENT title -> NEW title.
ROW_RENAME = {
    "Health & Overview": "SLI Summary",
    "Capacity & Quotas": "Capacity — Utilization & Saturation",
    "CPU": "Compute — Utilization",
    "Network & Disk": "Network & Disk — Utilization",
    "Protocol": "Protocol — Rate & Duration (RED)",
    "Per-Node": "Per-Node Detail",
}
ROW_ORDER = ["SLI Summary", "Capacity — Utilization & Saturation",
             "Compute — Utilization", "Network & Disk — Utilization",
             "Protocol — Rate & Duration (RED)", "Per-Node Detail"]
COLLAPSE = {"Per-Node Detail"}


def main():
    dash = load(OVERVIEW)
    rows = group_rows(dash["panels"])
    for r in rows:
        r["row"]["title"] = ROW_RENAME.get(r["row"]["title"], r["row"]["title"])
    by_title = {r["row"]["title"]: r for r in rows}
    assert set(by_title) == set(ROW_ORDER), set(by_title) ^ set(ROW_ORDER)
    dash["panels"] = emit_rows(by_title, ROW_ORDER, COLLAPSE)
    dump(OVERVIEW, dash)
    print(f"overview restructured: {len(ROW_ORDER)} rows, {len(COLLAPSE)} collapsed")


if __name__ == "__main__":
    main()
