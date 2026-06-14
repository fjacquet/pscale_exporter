"""Throwaway tooling for the dashboard-polish plan. Deleted in the final task."""
import json
from pathlib import Path

DASH_DIR = Path("grafana/provisioning/dashboards/json")
OVERVIEW = DASH_DIR / "powerscale-overview.json"
ADVANCED = DASH_DIR / "powerscale-advanced.json"


def load(path):
    with open(path) as f:
        return json.load(f)


def dump(path, obj):
    """Write with the repo's convention: 2-space indent + trailing newline."""
    with open(path, "w") as f:
        json.dump(obj, f, indent=2)
        f.write("\n")


def iter_panels(dash):
    """Yield every panel, descending into collapsed-row .panels arrays."""
    for p in dash.get("panels", []):
        yield p
        for child in p.get("panels", []):
            yield child


def group_rows(panels):
    """Normalize a panels list (flat or with nested rows) into ordered rows.

    Returns a list of {"row": <row panel>, "children": [<panel>...]}. Each child
    gets a transient "_off" = its gridPos.y minus its row's original gridPos.y,
    so callers can re-place children below a row at any new y without overlap.
    """
    flat = []
    for p in panels:
        if p.get("type") == "row":
            kids = p.pop("panels", [])
            p["collapsed"] = False
            flat.append(p)
            flat.extend(kids)
        else:
            flat.append(p)

    rows, current = [], None
    for p in flat:
        if p.get("type") == "row":
            current = {"row": p, "children": []}
            rows.append(current)
        else:
            assert current is not None, "panel before first row"
            current["children"].append(p)

    for r in rows:
        row_y = r["row"]["gridPos"]["y"]
        for c in r["children"]:
            c["_off"] = c["gridPos"]["y"] - row_y
    return rows


def emit_rows(rows_by_title, order, collapse):
    """Build a flat panels array from rows in `order`, recomputing gridPos.y.

    Collapsed rows nest their children (and consume only their 1-row header
    height on the board); expanded rows emit children as siblings. Returns the
    new panels list.
    """
    out, y = [], 0
    for title in order:
        r = rows_by_title[title]
        row_panel = r["row"]
        row_panel["gridPos"] = {"h": 1, "w": 24, "x": 0, "y": y}
        for c in r["children"]:
            c["gridPos"]["y"] = y + c.pop("_off")
        if title in collapse:
            row_panel["collapsed"] = True
            row_panel["panels"] = r["children"]
            out.append(row_panel)
            y += 1
        else:
            row_panel["collapsed"] = False
            row_panel["panels"] = []
            out.append(row_panel)
            out.extend(r["children"])
            bottoms = [c["gridPos"]["y"] + c["gridPos"]["h"] for c in r["children"]]
            y = max([y + 1] + bottoms)
    return out


def expr_set(dash):
    """Sorted list of every PromQL expr in the dashboard (order-independent)."""
    out = []

    def walk(node):
        if isinstance(node, dict):
            if "expr" in node and isinstance(node["expr"], str):
                out.append(node["expr"])
            for v in node.values():
                walk(v)
        elif isinstance(node, list):
            for v in node:
                walk(v)

    walk(dash)
    return sorted(out)
