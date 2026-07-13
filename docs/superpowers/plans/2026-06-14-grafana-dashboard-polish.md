# Grafana Dashboard Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure and polish the two bundled Grafana dashboards (Overview, Advanced) around RED/USE methods with per-panel descriptions, legend calcs, and threshold discipline — without changing any metric, query, or exporter code.

**Architecture:** All edits are data-driven Python transforms run against the dashboard JSON. A title-keyed metadata map supplies descriptions/units/thresholds; a structural step reorders rows and collapses designated rows. After every transform, a validation harness asserts the JSON is valid and the set of PromQL `expr` strings is byte-identical to a captured baseline (proving no query changed).

**Tech Stack:** Grafana schemaVersion 39 dashboard JSON (2-space indent, trailing newline), Python 3 stdlib (`json`), `jq`, MkDocs (strict build).

**Spec:** `docs/superpowers/specs/2026-06-14-grafana-dashboard-polish-design.md`

**Invariants (must hold after every task):**

- `powerscale-overview.json`: exactly **26** `expr` strings, set unchanged.
- `powerscale-advanced.json`: exactly **37** `expr` strings, set unchanged.
- Both files: valid JSON, 2-space indent, single trailing newline.

**Key directories:**

- Dashboards: `grafana/provisioning/dashboards/json/`
- Throwaway tooling: `scripts/dashtools/` (created in Task 1, **deleted in Task 8** — must not ship).

---

## Task 1: Validation harness + baseline capture

**Files:**

- Create: `scripts/dashtools/lib.py` (shared helpers)
- Create: `scripts/dashtools/check.py` (validator)
- Create: `scripts/dashtools/baseline/` (captured expr-sets)

- [ ] **Step 1: Create the shared library**

Create `scripts/dashtools/lib.py`:

```python
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
```

- [ ] **Step 2: Create the validator**

Create `scripts/dashtools/check.py`:

```python
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
```

- [ ] **Step 3: Capture the baseline expr-sets**

Run:

```bash
cd scripts/dashtools && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-overview.json --capture && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-advanced.json --capture && \
cd ../..
```

Expected:

```
CAPTURED 26 exprs -> baseline/powerscale-overview.exprs.json
CAPTURED 37 exprs -> baseline/powerscale-advanced.exprs.json
```

- [ ] **Step 4: Verify the validator passes on untouched files**

Run:

```bash
cd scripts/dashtools && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-overview.json && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-advanced.json && \
cd ../..
```

Expected:

```
OK .../powerscale-overview.json: 26 exprs unchanged
OK .../powerscale-advanced.json: 37 exprs unchanged
```

- [ ] **Step 5: Commit**

```bash
git add scripts/dashtools/
git commit -m "chore(dash): add throwaway validation harness + expr baseline"
```

---

## Task 2: Overview — per-panel metadata pass (descriptions, legends, thresholds)

**Files:**

- Create: `scripts/dashtools/apply_overview_meta.py`
- Modify: `grafana/provisioning/dashboards/json/powerscale-overview.json`

This task only changes `description`, timeseries legend/tooltip, and a small set of stat/gauge thresholds. No reordering. The transform is keyed by exact panel `title`, so it is safe to re-run and order-independent.

- [ ] **Step 1: Write the transform**

Create `scripts/dashtools/apply_overview_meta.py`:

```python
"""Apply descriptions, legend calcs, and thresholds to the Overview dashboard."""
from lib import OVERVIEW, load, dump, iter_panels

DESCRIPTIONS = {
    "Clusters Up": "Number of clusters scraping successfully (powerscale_up=1). Should equal your configured cluster count.",
    "Cluster Status": "Per-cluster scrape state; red means the last collection failed.",
    "OneFS API Version": "Detected OneFS platform API version negotiated at session start.",
    "Last Scrape Age": "Seconds since the last successful collection. Red indicates stale data (>2x the collection interval).",
    "NFS Exports": "Number of configured NFS exports.",
    "SMB Shares": "Number of configured SMB shares.",
    "Snapshots": "Total number of snapshots on the cluster.",
    "Capacity Used %": "Used /ifs capacity as a percent of total. Warning at 85%, critical at 95%.",
    "Cluster Capacity (used / available / total)": "Used, available, and total /ifs capacity over time.",
    "Top Quotas (usage vs hard limit)": "Quotas ranked by logical usage against their hard limit.",
    "Cluster CPU": "Cluster-aggregate CPU split into system, user, and idle.",
    "External Network Throughput": "External (front-end) inbound and outbound network throughput.",
    "Cluster Disk IOPS": "Cluster-wide disk transfer rate (ops/s). Aggregate with sum/avg, never rate().",
    "Protocol Operations": "Per-protocol operation rate (ops/s) by operation class.",
    "Protocol Latency": "Average per-protocol operation latency.",
    "Node CPU Idle": "Per-node idle CPU; low idle indicates CPU saturation.",
    "Node Memory Used": "Per-node memory in use.",
    "Node Disk IOPS": "Per-node disk transfer rate (ops/s).",
    "Node Used Capacity": "Per-node used /ifs capacity.",
}

# Panels that get directional thresholds (single-direction meaning only).
PERCENT_USED = {"steps": [
    {"color": "green", "value": None},
    {"color": "yellow", "value": 85},
    {"color": "red", "value": 95},
]}
STALENESS = {"steps": [
    {"color": "green", "value": None},
    {"color": "red", "value": 120},
]}
BOOL_BAD = {"steps": [  # 0 good, >0 bad
    {"color": "green", "value": None},
    {"color": "red", "value": 1},
]}
UP_GOOD = {"steps": [  # 0 bad, >=1 good
    {"color": "red", "value": None},
    {"color": "green", "value": 1},
]}

THRESHOLDS = {
    "Capacity Used %": PERCENT_USED,
    "Last Scrape Age": STALENESS,
    "Clusters Up": UP_GOOD,
    "Cluster Status": UP_GOOD,
}

LEGEND = {"displayMode": "table", "placement": "bottom",
          "calcs": ["lastNotNull", "max", "mean"]}
TOOLTIP = {"mode": "multi", "sort": "desc"}


def apply(dash):
    for p in iter_panels(dash):
        title = p.get("title")
        if title in DESCRIPTIONS:
            p["description"] = DESCRIPTIONS[title]
        fc = p.setdefault("fieldConfig", {}).setdefault("defaults", {})
        if title in THRESHOLDS:
            fc["thresholds"] = {"mode": "absolute", **THRESHOLDS[title]}
        if p.get("type") == "timeseries":
            opts = p.setdefault("options", {})
            opts["legend"] = LEGEND
            opts["tooltip"] = TOOLTIP


dash = load(OVERVIEW)
apply(dash)
dump(OVERVIEW, dash)
print("overview metadata applied")
```

- [ ] **Step 2: Run the transform**

Run:

```bash
cd scripts/dashtools && python3 apply_overview_meta.py && cd ../..
```

Expected: `overview metadata applied`

- [ ] **Step 3: Validate JSON + expr-set + description coverage**

Run:

```bash
jq empty grafana/provisioning/dashboards/json/powerscale-overview.json && \
cd scripts/dashtools && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-overview.json && \
python3 -c "from lib import OVERVIEW, load, iter_panels; d=load(OVERVIEW); n=[p for p in iter_panels(d) if p.get('type')!='row']; miss=[p['title'] for p in n if not p.get('description')]; print('missing desc:', miss); assert not miss" && \
cd ../..
```

Expected:

```
OK .../powerscale-overview.json: 26 exprs unchanged
missing desc: []
```

- [ ] **Step 4: Commit**

```bash
git add scripts/dashtools/apply_overview_meta.py grafana/provisioning/dashboards/json/powerscale-overview.json
git commit -m "feat(dash): add descriptions, legend calcs, thresholds to Overview"
```

---

## Task 3: Overview — RED/USE row restructure + collapse Per-Node

**Files:**

- Create: `scripts/dashtools/restructure_overview.py`
- Modify: `grafana/provisioning/dashboards/json/powerscale-overview.json`

Target row order and naming (see spec). The "Per-Node" row becomes collapsed: its 4 child panels move into the row panel's `panels` array, and `collapsed: true` is set. Row titles are renamed to carry the RED/USE intent. `gridPos.y` is recomputed top-to-bottom so panels do not overlap.

- [ ] **Step 1: Write the restructure transform**

Create `scripts/dashtools/restructure_overview.py`:

```python
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
```

> **gridPos handling:** `group_rows` records each child's vertical offset below its
> row; `emit_rows` re-places children at `new_row_y + offset`, so reordering and
> collapsing never overlap panels. The Overview row order here matches the source
> order (only the Per-Node row collapses), so this is mostly a rename + collapse;
> the same helpers handle the real reorder in Advanced (Task 5).

- [ ] **Step 2: Run the restructure**

Run:

```bash
cd scripts/dashtools && python3 restructure_overview.py && cd ../..
```

Expected: `overview restructured: 6 rows, 1 collapsed`

- [ ] **Step 3: Validate structure + expr-set + no overlapping panels**

Run:

```bash
jq empty grafana/provisioning/dashboards/json/powerscale-overview.json && \
jq -r '[.panels[] | select(.type=="row") | .title] | @json' grafana/provisioning/dashboards/json/powerscale-overview.json && \
jq '[.panels[] | select(.type=="row" and .collapsed==true) | {title, nested:(.panels|length)}]' grafana/provisioning/dashboards/json/powerscale-overview.json && \
cd scripts/dashtools && python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-overview.json && cd ../..
```

Expected: row titles in the new order; `Per-Node Detail` shows `nested: 4`; `OK ... 26 exprs unchanged`.

- [ ] **Step 4: Import-check in Grafana (manual)**

Import `powerscale-overview.json` into any Grafana ≥10 instance (Dashboards → New → Import → upload JSON). Confirm: no schema/plugin errors, rows render in RED/USE order, the "Per-Node Detail" row is collapsed and expands to 4 populated panels with no overlap. If panels overlap, apply the explicit-layout fallback from the Step 1 note, re-run, and re-validate.

- [ ] **Step 5: Commit**

```bash
git add scripts/dashtools/restructure_overview.py grafana/provisioning/dashboards/json/powerscale-overview.json
git commit -m "feat(dash): restructure Overview into RED/USE order, collapse Per-Node"
```

---

## Task 4: Advanced — per-panel metadata pass

**Files:**

- Create: `scripts/dashtools/apply_advanced_meta.py`
- Modify: `grafana/provisioning/dashboards/json/powerscale-advanced.json`

Same shape as Task 2. Provisional caveats live in the **description** here (per spec), so the row titles can be cleaned in Task 5.

- [ ] **Step 1: Write the transform**

Create `scripts/dashtools/apply_advanced_meta.py`:

```python
"""Apply descriptions, legend calcs, and thresholds to the Advanced dashboard."""
from lib import ADVANCED, load, dump, iter_panels

PROV = "Provisional: stat keys not yet live-validated against an OneFS cluster."

DESCRIPTIONS = {
    "Nodes Read-only": "Count of nodes mounted read-only (powerscale_node_readonly=1). Should be 0.",
    "Nodes Smartfailing": "Count of nodes smartfailing or smartfailed. Should be 0.",
    "Active Critical Events": "Unresolved OneFS event-group occurrences at critical severity.",
    "SyncIQ Policies Failed": "SyncIQ replication policies whose last run failed or needs attention.",
    "Drives by State": "Drive counts grouped by drive state (HEALTHY, SMARTFAIL, DEAD, ...).",
    "Active Events by Severity": "Unresolved OneFS event-group occurrences over time, by severity.",
    "SyncIQ Policies": "SyncIQ replication policies with enabled state and last-run result.",
    "Snapshot Space Used": "Aggregate space held by all snapshots.",
    "Cache Read Hit vs Miss (L1/L2/L3)": "L1/L2/L3 cache read hit vs miss throughput. " + PROV,
    "Cache Hit Ratio by Level": "Read-cache hit ratio per level. " + PROV,
    "Node CPU (sys / user / idle)": "Per-node CPU split into system, user, and idle.",
    "Quota Usage vs Thresholds": "Per-quota logical/physical usage against advisory, soft, and hard thresholds.",
    "Dedupe Logical Saved": "Logical space saved by deduplication. " + PROV,
    "Deduplicated Data": "Logical data that has been deduplicated. " + PROV,
    "Top Drive IOPS": "Highest per-drive operation rates. " + PROV,
    "Drive Busy %": "Per-drive busy time. " + PROV,
    "Client Operations by Protocol/Class": "Per-client operation rate by protocol and class. " + PROV,
    "Client Throughput (in / out)": "Per-client inbound/outbound throughput. " + PROV,
    "Power-Supply Failures": "Failed power supplies across nodes. Should be 0. " + PROV,
    "Node Temperature": "Node temperature sensor readings. " + PROV,
    "Fan Speed": "Node fan speed readings. " + PROV,
}

BOOL_BAD = {"mode": "absolute", "steps": [
    {"color": "green", "value": None},
    {"color": "red", "value": 1},
]}
THRESHOLDS = {
    "Nodes Read-only": BOOL_BAD,
    "Nodes Smartfailing": BOOL_BAD,
    "Active Critical Events": BOOL_BAD,
    "SyncIQ Policies Failed": BOOL_BAD,
    "Power-Supply Failures": BOOL_BAD,
}
LEGEND = {"displayMode": "table", "placement": "bottom",
          "calcs": ["lastNotNull", "max", "mean"]}
TOOLTIP = {"mode": "multi", "sort": "desc"}


def apply(dash):
    for p in iter_panels(dash):
        title = p.get("title")
        if title in DESCRIPTIONS:
            p["description"] = DESCRIPTIONS[title]
        fc = p.setdefault("fieldConfig", {}).setdefault("defaults", {})
        if title in THRESHOLDS:
            fc["thresholds"] = THRESHOLDS[title]
        if p.get("type") == "timeseries":
            opts = p.setdefault("options", {})
            opts["legend"] = LEGEND
            opts["tooltip"] = TOOLTIP


dash = load(ADVANCED)
apply(dash)
dump(ADVANCED, dash)
print("advanced metadata applied")
```

- [ ] **Step 2: Run the transform**

Run:

```bash
cd scripts/dashtools && python3 apply_advanced_meta.py && cd ../..
```

Expected: `advanced metadata applied`

- [ ] **Step 3: Validate JSON + expr-set + description coverage**

Run:

```bash
jq empty grafana/provisioning/dashboards/json/powerscale-advanced.json && \
cd scripts/dashtools && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-advanced.json && \
python3 -c "from lib import ADVANCED, load, iter_panels; d=load(ADVANCED); n=[p for p in iter_panels(d) if p.get('type')!='row']; miss=[p['title'] for p in n if not p.get('description')]; print('missing desc:', miss); assert not miss" && \
cd ../..
```

Expected: `OK ... 37 exprs unchanged` and `missing desc: []`.

- [ ] **Step 4: Commit**

```bash
git add scripts/dashtools/apply_advanced_meta.py grafana/provisioning/dashboards/json/powerscale-advanced.json
git commit -m "feat(dash): add descriptions, legend calcs, thresholds to Advanced"
```

---

## Task 5: Advanced — collapse provisional rows + clean titles

**Files:**

- Create: `scripts/dashtools/restructure_advanced.py`
- Modify: `grafana/provisioning/dashboards/json/powerscale-advanced.json`

Rename the five provisional rows to clean names (drop the parenthetical caveats now living in descriptions) and set them `collapsed: true` with children nested. Validated rows stay expanded and ordered first.

- [ ] **Step 1: Write the restructure transform**

Create `scripts/dashtools/restructure_advanced.py`:

```python
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
```

> **Row title check:** the `ROW_RENAME` keys must match the *exact* current row
> titles (em-dash `—`, not hyphen). Verify with
> `jq -r '.panels[]|select(.type=="row")|.title' grafana/provisioning/dashboards/json/powerscale-advanced.json`
> before running; the `assert set(by_title) == set(ROW_ORDER)` will fail loudly if any title differs.

- [ ] **Step 2: Run the restructure**

Run:

```bash
cd scripts/dashtools && python3 restructure_advanced.py && cd ../..
```

Expected: `advanced restructured: 9 rows, 5 collapsed`

- [ ] **Step 3: Validate structure + expr-set**

Run:

```bash
jq empty grafana/provisioning/dashboards/json/powerscale-advanced.json && \
jq '[.panels[] | select(.type=="row") | {title, collapsed, nested:(.panels|length)}]' grafana/provisioning/dashboards/json/powerscale-advanced.json && \
cd scripts/dashtools && python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-advanced.json && cd ../..
```

Expected: 9 rows; the 5 provisional rows show `collapsed: true` with `nested > 0`; `OK ... 37 exprs unchanged`.

- [ ] **Step 4: Import-check in Grafana (manual)**

Import `powerscale-advanced.json`. Confirm: validated rows render expanded and ordered first; the 5 provisional rows are collapsed; expanding each shows its panels with the provisional caveat in the panel ⓘ tooltip; no overlap.

- [ ] **Step 5: Commit**

```bash
git add scripts/dashtools/restructure_advanced.py grafana/provisioning/dashboards/json/powerscale-advanced.json
git commit -m "feat(dash): collapse provisional Advanced rows, clean titles"
```

---

## Task 6: Refresh dashboard docs

**Files:**

- Modify: `docs/dashboards.md`

- [ ] **Step 1: Read the current doc**

Run: `cat docs/dashboards.md`
Note the existing structure so the refresh matches the doc's voice and headings.

- [ ] **Step 2: Update the row/panel descriptions to match the new layout**

Edit `docs/dashboards.md` so that:

- The Overview section lists the new RED/USE row order (SLI Summary; Capacity — Utilization & Saturation; Compute — Utilization; Network & Disk — Utilization; Protocol — Rate & Duration (RED); Per-Node Detail *(collapsed)*).
- The Advanced section lists validated rows first (Cluster Health; Data Protection; Node CPU Detail; Quota Detail) then the 5 collapsed provisional rows (Cache Efficiency; Storage Efficiency; Per-Drive; Per-Client; Hardware), noting they are collapsed-by-default and provisional.
- Add one sentence near the top: "Every panel carries a description (hover the ⓘ); provisional panels note that their stat keys are not yet live-validated against OneFS."

Do not invent metric names — reference only those already in `docs/metrics.md`.

- [ ] **Step 3: Build the docs strictly**

Run:

```bash
uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict
```

Expected: build succeeds, no warnings (strict mode fails on broken links/refs).

- [ ] **Step 4: Commit**

```bash
git add docs/dashboards.md
git commit -m "docs: align dashboard guide with RED/USE restructure"
```

---

## Task 7: Full validation gate

**Files:** none (verification only)

- [ ] **Step 1: Re-validate both dashboards end-to-end**

Run:

```bash
jq empty grafana/provisioning/dashboards/json/powerscale-overview.json && \
jq empty grafana/provisioning/dashboards/json/powerscale-advanced.json && \
cd scripts/dashtools && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-overview.json && \
python3 check.py ../../grafana/provisioning/dashboards/json/powerscale-advanced.json && \
cd ../..
```

Expected: both `OK ... exprs unchanged` (26 and 37).

- [ ] **Step 2: Confirm every non-row panel has a description (both files)**

Run:

```bash
cd scripts/dashtools && python3 -c "
from lib import OVERVIEW, ADVANCED, load, iter_panels
for f in (OVERVIEW, ADVANCED):
    d = load(f)
    miss = [p.get('title') for p in iter_panels(d) if p.get('type')!='row' and not p.get('description')]
    print(f.name, 'missing:', miss)
    assert not miss
print('ALL PANELS DESCRIBED')
" && cd ../..
```

Expected: `ALL PANELS DESCRIBED`.

- [ ] **Step 3: Confirm collapsed-row counts**

Run:

```bash
for f in overview advanced; do echo -n "$f collapsed rows: "; \
jq '[.panels[] | select(.type=="row" and .collapsed==true)] | length' \
grafana/provisioning/dashboards/json/powerscale-$f.json; done
```

Expected: `overview collapsed rows: 1`, `advanced collapsed rows: 5`.

---

## Task 8: Remove throwaway tooling

**Files:**

- Delete: `scripts/dashtools/`

- [ ] **Step 1: Delete the tooling directory**

Run: `git rm -r scripts/dashtools`

- [ ] **Step 2: Confirm only dashboard JSON + docs remain changed vs main**

Run: `git diff --name-only main...HEAD`
Expected (no `scripts/dashtools` entries):

```
docs/dashboards.md
docs/superpowers/plans/2026-06-14-grafana-dashboard-polish.md
docs/superpowers/specs/2026-06-14-grafana-dashboard-polish-design.md
grafana/provisioning/dashboards/json/powerscale-advanced.json
grafana/provisioning/dashboards/json/powerscale-overview.json
```

- [ ] **Step 3: Commit**

```bash
git commit -m "chore(dash): remove throwaway validation tooling"
```

---

## Notes / rationale captured during planning

- **Thresholds are applied only where a single direction is meaningful** (Capacity Used %, Last Scrape Age staleness, boolean health stats). CPU timeseries mix sys/user/idle series, where a single graph-wide threshold would mislead — so they get legend calcs but no thresholds. This is a deliberate refinement of the spec's "CPU % green<70" line, which only holds for utilization, not idle.
- **All edits are title-keyed and idempotent**, so transforms can be re-run safely and a subagent executing tasks out of order still converges.
- **The expr-set baseline is the safety net**: it mechanically proves the guardrail "no metric/query change" after every single transform.

```
