"""Collapse provisional Advanced rows and clean their titles."""
from lib import ADVANCED, load, dump, group_rows, emit_rows

ROW_RENAME = {
    "Cache Efficiency (provisional keys — verify via statistics/keys)": "Cache Efficiency",
    "Storage Efficiency (provisional — dedupe-summary schema)": "Storage Efficiency",
    "Per-Drive (provisional — summary/drive schema)": "Per-Drive",
    "Per-Client (provisional — summary/client schema)": "Per-Client",
    "Hardware (provisional — node status/sensors)": "Hardware",
}
# Final top-level row order: validated first, provisional (collapsed) last.
ROW_ORDER = [
    "Cluster Health", "Data Protection", "Node CPU Detail", "Quota Detail",
    "Cache Efficiency", "Storage Efficiency", "Per-Drive", "Per-Client", "Hardware",
]
COLLAPSE = {"Cache Efficiency", "Storage Efficiency", "Per-Drive",
            "Per-Client", "Hardware"}


def main():
    dash = load(ADVANCED)
    rows = group_rows(dash["panels"])
    for r in rows:
        t = r["row"]["title"]
        r["row"]["title"] = ROW_RENAME.get(t, t)
    by_title = {r["row"]["title"]: r for r in rows}
    assert set(by_title) == set(ROW_ORDER), set(by_title) ^ set(ROW_ORDER)
    dash["panels"] = emit_rows(by_title, ROW_ORDER, COLLAPSE)
    dump(ADVANCED, dash)
    print(f"advanced restructured: {len(ROW_ORDER)} rows, {len(COLLAPSE)} collapsed")


if __name__ == "__main__":
    main()
