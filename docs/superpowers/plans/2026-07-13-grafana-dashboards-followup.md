# Grafana Dashboards Follow-up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Grafana panels visualizing the three new metric families (license, storagepool, workload) shipped by PRs #35/#36/#37 — the metrics currently have exporter + docs coverage but no dashboards.

**Architecture:** Purely additive dashboard-JSON edits. New rows are **appended at the end** of each existing board's `panels` array (no existing panel's `gridPos` or `id` changes), and one brand-new board is created. Each task = one metric family's panels + its validation gate, ending independently testable. No Go code changes.

**Tech Stack:** Grafana `schemaVersion 39` dashboard JSON, Python 3 stdlib (`json.tool`) for validation, `grep`/`comm` for the metric cross-check, MkDocs (strict build) for docs.

## Global Constraints

- **Metric names are fixed** — every `powerscale_*` metric a panel references MUST exist verbatim in `internal/powerscale/derivations.go`. The three families:
  - `powerscale_license_info` (labels `cluster,cluster_id,name,status`), `powerscale_license_expired` (`…,name`), `powerscale_license_days_to_expiry` (`…,name`; emitted only for licenses that expire).
  - `powerscale_storagepool_{total,used,available}_capacity_bytes` and `powerscale_storagepool_{ssd,hdd}_{total,used,available}_capacity_bytes` (labels `cluster,cluster_id,pool,type`; `type` ∈ `nodepool|tier`).
  - `powerscale_workload_operations_per_second`, `…_in_bytes_per_second`, `…_out_bytes_per_second`, `…_cpu_microseconds_per_second` (labels `cluster,cluster_id,node,zone,protocol,username,system_name,job_type`).
- **`schemaVersion` = 39; `uid` = filename base.** New board `uid` = `powerscale-workloads` (must not collide with `powerscale-overview`, `powerscale-advanced`, `powerscale-capacity-sla`, `rYdddlPWk`).
- **Tags start** `["powerscale", "onefs", "dell", …]`.
- **Per-second gauges** (workload ops/throughput/cpu, and any bytes/sec) are read directly with `sum`/`avg` — **never `rate()`**.
- **New content ships EXPANDED** (`collapsed: false`, no "provisional" caveat) — a deliberate departure from the collapsed-provisional convention used by the Cache/Per-Drive rows.
- **Datasource reference** in every panel/target: `{ "type": "prometheus", "uid": "${datasource}" }`.
- **Append-only, style-matched.** `powerscale-overview.json` is canonical `json.dump(indent=2)` **expanded** style — its appended fragment uses that style. `powerscale-capacity-sla.json` is **inline-compact** style (`{ "h": 1, … }` on one line) — its fragments use that style. Never re-serialize a whole file. `powerscale-workloads.json` is new → authored in the inline-compact style (matches capacity-sla, the newest board).
- **Panel `id`s are unique per board.** Next free ids: `powerscale-capacity-sla` → 19; `powerscale-overview` → 26; `powerscale-workloads` → starts at 1.

---

## File Structure

- **Modify** `grafana/provisioning/dashboards/json/powerscale-capacity-sla.json`
  - Task 1 appends a **"Storage Pools — Capacity"** row: ids 19 (row), 20 (table), 21 (used-% timeseries), 22 (SSD/HDD timeseries), at `y` 46–64.
  - Task 2 appends a **"Licensing"** row: ids 23 (row), 24 (min-days stat), 25 (expired stat), at `y` 64–69.
- **Modify** `grafana/provisioning/dashboards/json/powerscale-overview.json`
  - Task 2 appends a **"Licensing"** row: ids 26 (row), 27 (status table), 28 (days-to-expiry table), at `y` 54–63.
- **Create** `grafana/provisioning/dashboards/json/powerscale-workloads.json`
  - Task 3: new board `uid powerscale-workloads`; vars datasource/cluster/zone/protocol/username; one expanded row with a prereq text note + 3 timeseries + 1 snapshot table (ids 1–6).
- **Modify** `docs/dashboards.md`
  - Task 4: document all new content.

**Provisioning:** `grafana/provisioning/dashboards/dashboards.yml` globs the whole `json/` folder — the new board auto-loads, **no config change**.

**Validation vocabulary (used verbatim in every task):**

- *JSON-valid gate* — `python3 -m json.tool <file> >/dev/null` exits 0.
- *Metric cross-check* — this prints nothing when every family metric referenced in the dashboards exists in the code:
  ```bash
  comm -23 \
    <(grep -rhoE 'powerscale_(license|storagepool|workload)_[a-z_]+' grafana/provisioning/dashboards/json/ | sort -u) \
    <(grep -hoE  'powerscale_(license|storagepool|workload)_[a-z_]+' internal/powerscale/derivations.go | sort -u)
  ```

---

## Task 1: Storage-pool capacity row (Capacity & SLA board)

**Files:**
- Modify: `grafana/provisioning/dashboards/json/powerscale-capacity-sla.json` (append before the panels-array close)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: leaves the capacity-sla panels array ending in a `timeseries` panel (id 22) whose close is `    }` — Task 2 appends after it using the same anchor.

**Append mechanism:** the board's panels array closes with these three lines (the last panel's close, the array close, then `"refresh"`), which is the unique anchor:
```
    }
  ],
  "refresh": "1m",
```

- [ ] **Step 1: Confirm the file currently parses (baseline gate)**

Run: `python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-capacity-sla.json >/dev/null && echo OK`
Expected: `OK`

- [ ] **Step 2: Append the Storage Pools row**

Using the Edit tool on `grafana/provisioning/dashboards/json/powerscale-capacity-sla.json`, replace the anchor:

old_string:
```
    }
  ],
  "refresh": "1m",
```

new_string:
```
    },
    {
      "type": "row",
      "id": 19,
      "title": "Storage Pools — Capacity",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 46 },
      "panels": []
    },
    {
      "type": "table",
      "id": 20,
      "title": "Pool Capacity (node pools & tiers)",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 9, "w": 24, "x": 0, "y": 47 },
      "fieldConfig": {
        "defaults": {
          "custom": { "align": "auto", "filterable": true },
          "unit": "bytes"
        },
        "overrides": [
          {
            "matcher": { "id": "byName", "options": "used %" },
            "properties": [
              { "id": "unit", "value": "percent" },
              { "id": "decimals", "value": 1 },
              { "id": "custom.cellOptions", "value": { "type": "gauge", "mode": "gradient" } },
              { "id": "max", "value": 100 },
              {
                "id": "thresholds",
                "value": {
                  "mode": "absolute",
                  "steps": [
                    { "color": "green", "value": null },
                    { "color": "yellow", "value": 75 },
                    { "color": "red", "value": 90 }
                  ]
                }
              }
            ]
          }
        ]
      },
      "options": {
        "showHeader": true,
        "footer": { "show": false },
        "sortBy": [{ "displayName": "used %", "desc": true }]
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "100 * powerscale_storagepool_used_capacity_bytes{cluster=~\"$cluster\"} / powerscale_storagepool_total_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "A" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_used_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "B" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_total_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "C" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_available_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "D" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_ssd_used_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "E" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_ssd_total_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "F" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_hdd_used_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "G" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_hdd_total_capacity_bytes{cluster=~\"$cluster\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "H" }
      ],
      "transformations": [
        { "id": "merge", "options": {} },
        {
          "id": "organize",
          "options": {
            "excludeByName": { "Time": true, "__name__": true, "cluster_id": true, "instance": true, "job": true },
            "indexByName": {},
            "renameByName": {
              "Value #A": "used %",
              "Value #B": "used",
              "Value #C": "total",
              "Value #D": "available",
              "Value #E": "SSD used",
              "Value #F": "SSD total",
              "Value #G": "HDD used",
              "Value #H": "HDD total",
              "pool": "pool",
              "type": "type"
            }
          }
        }
      ],
      "description": "Per node pool and tier capacity: aggregate used %, used/total/available, and the SSD vs HDD media split. The list holds both node pools and tiers (a tier is the sum of its child node pools), distinguished by the type column — filter type=nodepool for a non-overlapping cluster total."
    },
    {
      "type": "timeseries",
      "id": 21,
      "title": "Node-Pool Used % (trend)",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 56 },
      "fieldConfig": {
        "defaults": {
          "custom": { "drawStyle": "line", "fillOpacity": 10, "lineWidth": 1, "showPoints": "never", "stacking": { "mode": "none" } },
          "unit": "percent",
          "min": 0,
          "max": 100
        },
        "overrides": []
      },
      "options": {
        "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull", "max"] },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "100 * powerscale_storagepool_used_capacity_bytes{cluster=~\"$cluster\", type=\"nodepool\"} / powerscale_storagepool_total_capacity_bytes{cluster=~\"$cluster\", type=\"nodepool\"}", "legendFormat": "{{cluster}} / {{pool}}", "refId": "A" }
      ],
      "description": "Used % over time per node pool (type=nodepool only, so tiers don't double-count their child pools). Thresholds: yellow at 75%, red at 90%."
    },
    {
      "type": "timeseries",
      "id": 22,
      "title": "SSD vs HDD Available (per pool)",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 56 },
      "fieldConfig": {
        "defaults": {
          "custom": { "drawStyle": "line", "fillOpacity": 10, "lineWidth": 1, "showPoints": "never", "stacking": { "mode": "none" } },
          "unit": "bytes"
        },
        "overrides": []
      },
      "options": {
        "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull", "min"] },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_ssd_available_capacity_bytes{cluster=~\"$cluster\"}", "legendFormat": "{{pool}} SSD", "refId": "A" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_storagepool_hdd_available_capacity_bytes{cluster=~\"$cluster\"}", "legendFormat": "{{pool}} HDD", "refId": "B" }
      ],
      "description": "Available capacity on SSD vs HDD media per pool — surfaces tiering/cache-media headroom the aggregate figure hides. An all-HDD pool reports SSD=0."
    }
  ],
  "refresh": "1m",
```

- [ ] **Step 3: JSON-valid gate**

Run: `python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-capacity-sla.json >/dev/null && echo OK`
Expected: `OK` (a syntax slip — missing comma, stray brace — fails here)

- [ ] **Step 4: Metric cross-check**

Run:
```bash
comm -23 \
  <(grep -rhoE 'powerscale_(license|storagepool|workload)_[a-z_]+' grafana/provisioning/dashboards/json/ | sort -u) \
  <(grep -hoE  'powerscale_(license|storagepool|workload)_[a-z_]+' internal/powerscale/derivations.go | sort -u)
```
Expected: no output (every referenced storagepool metric exists in the code).

- [ ] **Step 5: Confirm ids didn't collide and gridPos didn't disturb existing panels**

Run: `git diff --stat grafana/provisioning/dashboards/json/powerscale-capacity-sla.json`
Expected: only insertions, 0 deletions (purely additive).

- [ ] **Step 6: Commit**

```bash
git add grafana/provisioning/dashboards/json/powerscale-capacity-sla.json
git commit -m "feat(dashboards): add Storage Pools capacity row to Capacity & SLA board"
```

---

## Task 2: Licensing rows (Capacity & SLA + Overview boards)

**Files:**
- Modify: `grafana/provisioning/dashboards/json/powerscale-capacity-sla.json` (append a Licensing row — 2 stats)
- Modify: `grafana/provisioning/dashboards/json/powerscale-overview.json` (append a Licensing row — 2 tables)

**Interfaces:**
- Consumes: capacity-sla now ends in the Task 1 SSD/HDD timeseries (id 22); this task appends after it via the same anchor. Overview is untouched by Task 1.
- Produces: nothing later tasks consume.

- [ ] **Step 1: Baseline gate (both files parse)**

Run:
```bash
python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-capacity-sla.json >/dev/null && \
python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-overview.json >/dev/null && echo OK
```
Expected: `OK`

- [ ] **Step 2: Append the Licensing row to Capacity & SLA (inline style)**

Edit `grafana/provisioning/dashboards/json/powerscale-capacity-sla.json`, replace anchor:

old_string:
```
    }
  ],
  "refresh": "1m",
```

new_string:
```
    },
    {
      "type": "row",
      "id": 23,
      "title": "Licensing",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 64 },
      "panels": []
    },
    {
      "type": "stat",
      "id": 24,
      "title": "Min Days to License Expiry",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 65 },
      "fieldConfig": {
        "defaults": {
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "red", "value": null },
              { "color": "yellow", "value": 30 },
              { "color": "green", "value": 90 }
            ]
          },
          "unit": "d"
        },
        "overrides": []
      },
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "min(powerscale_license_days_to_expiry{cluster=~\"$cluster\"})", "legendFormat": "min days", "refId": "A" }
      ],
      "description": "Fewest days to expiry across all expiring licenses on the selected clusters. Perpetual licenses carry no expiry and are excluded. Red under 30 days, yellow under 90; shows No data when nothing expires."
    },
    {
      "type": "stat",
      "id": 25,
      "title": "Licenses Expired",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 65 },
      "fieldConfig": {
        "defaults": {
          "mappings": [],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "red", "value": 1 }
            ]
          },
          "unit": "none"
        },
        "overrides": []
      },
      "options": {
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto",
        "orientation": "auto",
        "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
        "textMode": "auto"
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "sum(powerscale_license_expired{cluster=~\"$cluster\"}) or vector(0)", "legendFormat": "expired", "refId": "A" }
      ],
      "description": "Count of licensed features currently marked expired on the selected clusters. Should be 0."
    }
  ],
  "refresh": "1m",
```

- [ ] **Step 3: Append the Licensing row to Overview (expanded style)**

Edit `grafana/provisioning/dashboards/json/powerscale-overview.json`, replace the end-of-file anchor:

old_string:
```
    }
  ]
}
```

new_string:
```
    },
    {
      "type": "row",
      "id": 26,
      "title": "Licensing",
      "collapsed": false,
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 54
      },
      "panels": []
    },
    {
      "type": "table",
      "id": 27,
      "title": "License Status",
      "datasource": {
        "type": "prometheus",
        "uid": "${datasource}"
      },
      "gridPos": {
        "h": 8,
        "w": 12,
        "x": 0,
        "y": 55
      },
      "fieldConfig": {
        "defaults": {
          "custom": {
            "align": "auto",
            "filterable": true
          },
          "unit": "none"
        },
        "overrides": [
          {
            "matcher": {
              "id": "byName",
              "options": "status"
            },
            "properties": [
              {
                "id": "mappings",
                "value": [
                  { "type": "value", "options": { "Licensed": { "color": "green", "index": 0 } } },
                  { "type": "value", "options": { "Activated": { "color": "green", "index": 1 } } },
                  { "type": "value", "options": { "Evaluation": { "color": "yellow", "index": 2 } } },
                  { "type": "value", "options": { "Expired": { "color": "red", "index": 3 } } }
                ]
              },
              {
                "id": "custom.cellOptions",
                "value": { "type": "color-text" }
              }
            ]
          }
        ]
      },
      "options": {
        "showHeader": true,
        "footer": {
          "show": false
        }
      },
      "targets": [
        {
          "datasource": {
            "type": "prometheus",
            "uid": "${datasource}"
          },
          "editorMode": "code",
          "expr": "powerscale_license_info{cluster=~\"$cluster\"}",
          "format": "table",
          "instant": true,
          "legendFormat": "__auto",
          "refId": "A"
        }
      ],
      "transformations": [
        {
          "id": "organize",
          "options": {
            "excludeByName": {
              "Time": true,
              "Value": true,
              "__name__": true,
              "cluster_id": true,
              "instance": true,
              "job": true
            },
            "indexByName": {},
            "renameByName": {
              "cluster": "cluster",
              "name": "feature",
              "status": "status"
            }
          }
        }
      ],
      "description": "Per-feature license status reported by OneFS. Licensed/Activated show green, Evaluation yellow, Expired red."
    },
    {
      "type": "table",
      "id": 28,
      "title": "Days to Expiry",
      "datasource": {
        "type": "prometheus",
        "uid": "${datasource}"
      },
      "gridPos": {
        "h": 8,
        "w": 12,
        "x": 12,
        "y": 55
      },
      "fieldConfig": {
        "defaults": {
          "custom": {
            "align": "auto",
            "filterable": true
          },
          "unit": "d"
        },
        "overrides": [
          {
            "matcher": {
              "id": "byName",
              "options": "days to expiry"
            },
            "properties": [
              {
                "id": "custom.cellOptions",
                "value": { "type": "color-text" }
              },
              {
                "id": "thresholds",
                "value": {
                  "mode": "absolute",
                  "steps": [
                    { "color": "red", "value": null },
                    { "color": "yellow", "value": 30 },
                    { "color": "green", "value": 90 }
                  ]
                }
              }
            ]
          }
        ]
      },
      "options": {
        "showHeader": true,
        "footer": {
          "show": false
        },
        "sortBy": [
          {
            "displayName": "days to expiry",
            "desc": false
          }
        ]
      },
      "targets": [
        {
          "datasource": {
            "type": "prometheus",
            "uid": "${datasource}"
          },
          "editorMode": "code",
          "expr": "powerscale_license_days_to_expiry{cluster=~\"$cluster\"}",
          "format": "table",
          "instant": true,
          "legendFormat": "__auto",
          "refId": "A"
        }
      ],
      "transformations": [
        {
          "id": "organize",
          "options": {
            "excludeByName": {
              "Time": true,
              "__name__": true,
              "cluster_id": true,
              "instance": true,
              "job": true
            },
            "indexByName": {},
            "renameByName": {
              "Value": "days to expiry",
              "cluster": "cluster",
              "name": "feature"
            }
          }
        }
      ],
      "description": "Days remaining until each expiring license lapses (perpetual licenses are omitted at the source). Sorted soonest-first; red under 30 days, yellow under 90."
    }
  ]
}
```

- [ ] **Step 4: JSON-valid gate (both files)**

Run:
```bash
python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-capacity-sla.json >/dev/null && \
python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-overview.json >/dev/null && echo OK
```
Expected: `OK`

- [ ] **Step 5: Metric cross-check**

Run:
```bash
comm -23 \
  <(grep -rhoE 'powerscale_(license|storagepool|workload)_[a-z_]+' grafana/provisioning/dashboards/json/ | sort -u) \
  <(grep -hoE  'powerscale_(license|storagepool|workload)_[a-z_]+' internal/powerscale/derivations.go | sort -u)
```
Expected: no output.

- [ ] **Step 6: Additive-only confirmation**

Run: `git diff --stat grafana/provisioning/dashboards/json/powerscale-capacity-sla.json grafana/provisioning/dashboards/json/powerscale-overview.json`
Expected: insertions only, 0 deletions.

- [ ] **Step 7: Commit**

```bash
git add grafana/provisioning/dashboards/json/powerscale-capacity-sla.json grafana/provisioning/dashboards/json/powerscale-overview.json
git commit -m "feat(dashboards): add Licensing rows to Overview and Capacity & SLA boards"
```

---

## Task 3: New Workloads board

**Files:**
- Create: `grafana/provisioning/dashboards/json/powerscale-workloads.json`

**Interfaces:**
- Consumes: nothing.
- Produces: a new provisioned board, `uid powerscale-workloads`.

- [ ] **Step 1: Create the file**

Write `grafana/provisioning/dashboards/json/powerscale-workloads.json` with exactly this content:

```json
{
  "annotations": {
    "list": [
      {
        "builtIn": 1,
        "datasource": { "type": "grafana", "uid": "-- Grafana --" },
        "enable": true,
        "hide": true,
        "iconColor": "rgba(0, 211, 255, 1)",
        "name": "Annotations & Alerts",
        "type": "dashboard"
      }
    ]
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "links": [
    {
      "asDropdown": false,
      "icon": "external link",
      "includeVars": true,
      "keepTime": true,
      "tags": [],
      "targetBlank": false,
      "title": "PowerScale / OneFS Overview",
      "tooltip": "",
      "type": "link",
      "url": "/d/powerscale-overview"
    }
  ],
  "liveNow": false,
  "panels": [
    {
      "type": "row",
      "id": 1,
      "title": "Per-Workload Performance",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "panels": []
    },
    {
      "type": "text",
      "id": 2,
      "title": "",
      "gridPos": { "h": 3, "w": 24, "x": 0, "y": 1 },
      "options": {
        "mode": "markdown",
        "content": "**Requires a configured OneFS performance dataset** (`isi performance datasets`). Without one this board is empty. Cardinality is governed by your dataset definition — use the **zone / protocol / username** variables above to slice when a broad dataset produces many series."
      },
      "description": "Prerequisite and usage note for the workload metrics."
    },
    {
      "type": "timeseries",
      "id": 3,
      "title": "Operations / sec per workload",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 },
      "fieldConfig": {
        "defaults": {
          "custom": { "drawStyle": "line", "fillOpacity": 10, "lineWidth": 1, "showPoints": "never", "stacking": { "mode": "none" } },
          "unit": "ops"
        },
        "overrides": []
      },
      "options": {
        "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull", "max"] },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_operations_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "legendFormat": "{{zone}}/{{protocol}}/{{username}}/{{job_type}} node{{node}}", "refId": "A" }
      ],
      "description": "Per-workload operations per second. A per-second gauge — aggregate with sum/avg, never rate(). Slice with the zone/protocol/username variables to bound the series count."
    },
    {
      "type": "timeseries",
      "id": 4,
      "title": "Throughput per workload (in / out)",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 },
      "fieldConfig": {
        "defaults": {
          "custom": { "drawStyle": "line", "fillOpacity": 10, "lineWidth": 1, "showPoints": "never", "stacking": { "mode": "none" } },
          "unit": "Bps"
        },
        "overrides": []
      },
      "options": {
        "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull", "max"] },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_in_bytes_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "legendFormat": "in {{zone}}/{{protocol}}/{{username}}", "refId": "A" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_out_bytes_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "legendFormat": "out {{zone}}/{{protocol}}/{{username}}", "refId": "B" }
      ],
      "description": "Per-workload read (in) and write (out) throughput. Per-second gauges — aggregate with sum/avg, never rate()."
    },
    {
      "type": "timeseries",
      "id": 5,
      "title": "CPU µs/sec per workload",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 12 },
      "fieldConfig": {
        "defaults": {
          "custom": { "drawStyle": "line", "fillOpacity": 10, "lineWidth": 1, "showPoints": "never", "stacking": { "mode": "none" } },
          "unit": "µs"
        },
        "overrides": []
      },
      "options": {
        "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull", "max"] },
        "tooltip": { "mode": "multi", "sort": "desc" }
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_cpu_microseconds_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "legendFormat": "{{zone}}/{{protocol}}/{{username}}/{{job_type}} node{{node}}", "refId": "A" }
      ],
      "description": "Per-workload CPU consumed, in microseconds of CPU per second across all cores. A per-second busy-ness rate, not a cumulative counter — aggregate with sum/avg, never rate()."
    },
    {
      "type": "table",
      "id": 6,
      "title": "Workload Snapshot",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 9, "w": 24, "x": 0, "y": 20 },
      "fieldConfig": {
        "defaults": { "custom": { "align": "auto", "filterable": true }, "unit": "none" },
        "overrides": [
          { "matcher": { "id": "byName", "options": "in B/s" }, "properties": [ { "id": "unit", "value": "Bps" } ] },
          { "matcher": { "id": "byName", "options": "out B/s" }, "properties": [ { "id": "unit", "value": "Bps" } ] },
          { "matcher": { "id": "byName", "options": "cpu µs/s" }, "properties": [ { "id": "unit", "value": "µs" } ] }
        ]
      },
      "options": {
        "showHeader": true,
        "footer": { "show": false },
        "sortBy": [{ "displayName": "ops/s", "desc": true }]
      },
      "targets": [
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_operations_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "A" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_in_bytes_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "B" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_out_bytes_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "C" },
        { "datasource": { "type": "prometheus", "uid": "${datasource}" }, "editorMode": "code", "expr": "powerscale_workload_cpu_microseconds_per_second{cluster=~\"$cluster\", zone=~\"$zone\", protocol=~\"$protocol\", username=~\"$username\"}", "format": "table", "instant": true, "legendFormat": "__auto", "refId": "D" }
      ],
      "transformations": [
        { "id": "merge", "options": {} },
        {
          "id": "organize",
          "options": {
            "excludeByName": { "Time": true, "__name__": true, "cluster_id": true, "instance": true, "job": true, "system_name": true },
            "indexByName": {},
            "renameByName": {
              "Value #A": "ops/s",
              "Value #B": "in B/s",
              "Value #C": "out B/s",
              "Value #D": "cpu µs/s",
              "cluster": "cluster",
              "node": "node",
              "zone": "zone",
              "protocol": "protocol",
              "username": "username",
              "job_type": "job_type"
            }
          }
        }
      ],
      "description": "Instant snapshot of every workload row (filtered by the variables), sorted by ops/s. Merges ops, throughput, and CPU per workload. Unpinned dimensions show as empty; system_name is hidden to keep the table readable."
    }
  ],
  "refresh": "30s",
  "schemaVersion": 39,
  "style": null,
  "tags": ["powerscale", "onefs", "dell", "workload", "performance"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 0,
        "includeAll": false,
        "label": "Data source",
        "multi": false,
        "name": "datasource",
        "options": [],
        "query": "prometheus",
        "refresh": 1,
        "regex": "",
        "skipUrlSync": false,
        "type": "datasource"
      },
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "${datasource}" },
        "definition": "label_values(powerscale_up, cluster)",
        "hide": 0,
        "includeAll": true,
        "label": "Cluster",
        "multi": true,
        "name": "cluster",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(powerscale_up, cluster)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "sort": 1,
        "type": "query"
      },
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "${datasource}" },
        "definition": "label_values(powerscale_workload_operations_per_second{cluster=~\"$cluster\"}, zone)",
        "hide": 0,
        "includeAll": true,
        "label": "Zone",
        "multi": true,
        "name": "zone",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(powerscale_workload_operations_per_second{cluster=~\"$cluster\"}, zone)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "sort": 1,
        "type": "query"
      },
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "${datasource}" },
        "definition": "label_values(powerscale_workload_operations_per_second{cluster=~\"$cluster\"}, protocol)",
        "hide": 0,
        "includeAll": true,
        "label": "Protocol",
        "multi": true,
        "name": "protocol",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(powerscale_workload_operations_per_second{cluster=~\"$cluster\"}, protocol)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "sort": 1,
        "type": "query"
      },
      {
        "current": {},
        "datasource": { "type": "prometheus", "uid": "${datasource}" },
        "definition": "label_values(powerscale_workload_operations_per_second{cluster=~\"$cluster\"}, username)",
        "hide": 0,
        "includeAll": true,
        "label": "Username",
        "multi": true,
        "name": "username",
        "options": [],
        "query": { "qryType": 1, "query": "label_values(powerscale_workload_operations_per_second{cluster=~\"$cluster\"}, username)", "refId": "PrometheusVariableQueryEditor-VariableQuery" },
        "refresh": 2,
        "regex": "",
        "sort": 1,
        "type": "query"
      }
    ]
  },
  "time": { "from": "now-6h", "to": "now" },
  "timezone": "",
  "title": "PowerScale / OneFS Workloads",
  "uid": "powerscale-workloads",
  "version": 1,
  "weekStart": ""
}
```

- [ ] **Step 2: JSON-valid gate**

Run: `python3 -m json.tool grafana/provisioning/dashboards/json/powerscale-workloads.json >/dev/null && echo OK`
Expected: `OK`

- [ ] **Step 3: uid uniqueness + presence**

Run:
```bash
python3 - <<'PY'
import json, glob, collections
uids = [json.load(open(f))["uid"] for f in glob.glob("grafana/provisioning/dashboards/json/*.json")]
dup = [u for u, c in collections.Counter(uids).items() if c > 1]
assert not dup, f"duplicate uids: {dup}"
assert "powerscale-workloads" in uids, "workloads board missing"
print("uids ok:", sorted(uids))
PY
```
Expected: `uids ok: ['powerscale-advanced', 'powerscale-capacity-sla', 'powerscale-overview', 'powerscale-workloads', 'rYdddlPWk']`

- [ ] **Step 4: Metric cross-check**

Run:
```bash
comm -23 \
  <(grep -rhoE 'powerscale_(license|storagepool|workload)_[a-z_]+' grafana/provisioning/dashboards/json/ | sort -u) \
  <(grep -hoE  'powerscale_(license|storagepool|workload)_[a-z_]+' internal/powerscale/derivations.go | sort -u)
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add grafana/provisioning/dashboards/json/powerscale-workloads.json
git commit -m "feat(dashboards): add per-workload performance board (powerscale-workloads)"
```

---

## Task 4: Document the new panels + final validation

**Files:**
- Modify: `docs/dashboards.md`

**Interfaces:**
- Consumes: the three boards' new content (Tasks 1–3).
- Produces: user-facing documentation.

- [ ] **Step 1: Add the Workloads board to the dashboard table**

Edit `docs/dashboards.md`, replace:
```
| **PowerScale / OneFS Capacity & SLA** | `powerscale-capacity-sla` | Availability/latency SLIs and capacity headroom with a days-to-full forecast. |
```
with:
```
| **PowerScale / OneFS Capacity & SLA** | `powerscale-capacity-sla` | Availability/latency SLIs and capacity headroom with a days-to-full forecast. |
| **PowerScale / OneFS Workloads** | `powerscale-workloads` | Per-workload operations, throughput and CPU. Requires a configured OneFS performance dataset. |
```

Also change the intro line `Three ready-made Grafana dashboards ship in the repo under` to `Four ready-made Grafana dashboards ship in the repo under`.

- [ ] **Step 2: Document the new Overview and Capacity & SLA rows**

Edit `docs/dashboards.md`. In the **Overview dashboard** rows list (the bulleted list under "One comprehensive board with these rows"), append this bullet:
```
- **Licensing** — per-feature license status (colored) and a days-to-expiry table (soonest first; red under 30 days).
```

In the **Capacity & SLA dashboard** section's bulleted list (the one ending with the quota table bullet), append:
```
- **Storage Pools — Capacity** — per node-pool/tier table (used %, used/total/available, and the SSD vs HDD media split), a node-pool used-% trend, and SSD-vs-HDD available capacity. The list holds both node pools and tiers; filter `type="nodepool"` for a non-overlapping cluster total.
- **Licensing** — min days to license expiry and a count of expired licenses.
```

- [ ] **Step 3: Add a Workloads dashboard section**

Edit `docs/dashboards.md`. Immediately before the `## Auto-provisioned (compose stack)` heading, insert:
```
## Workloads dashboard

Per-workload performance from OneFS statistics summaries (`uid` `powerscale-workloads`):

- **Operations/sec, Throughput (in/out), and CPU µs/sec** timeseries, one series per workload, plus a **Workload Snapshot** table (instant, sorted by ops/sec).
- Template variables **cluster / zone / protocol / username** slice the view — use them to bound the series count on a broad dataset.

!!! warning "Requires a performance dataset"
    Workload rows are produced only when a OneFS **performance dataset** is configured
    (`isi performance datasets`). Without one, this board is empty — every other dashboard is
    unaffected. Cardinality is governed by your dataset definition; the exporter exposes a
    fixed label set (`node`, `zone`, `protocol`, `username`, `system_name`, `job_type`) and
    omits the unbounded dimensions (`path`, IPs, SIDs).

!!! note "Per-second gauges"
    Operations, throughput and CPU are per-second gauges — aggregate with `sum`/`avg`, never
    `rate()`.
```

- [ ] **Step 4: MkDocs strict build**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict 2>&1 | tail -5`
Expected: build succeeds ("Documentation built in …s"). Pre-existing "not in nav" INFO lines are normal; any `ERROR` (bad link/anchor) must be fixed.

- [ ] **Step 5: Full cross-board validation**

Run:
```bash
for f in grafana/provisioning/dashboards/json/*.json; do python3 -m json.tool "$f" >/dev/null && echo "OK $f"; done
comm -23 \
  <(grep -rhoE 'powerscale_(license|storagepool|workload)_[a-z_]+' grafana/provisioning/dashboards/json/ | sort -u) \
  <(grep -hoE  'powerscale_(license|storagepool|workload)_[a-z_]+' internal/powerscale/derivations.go | sort -u)
```
Expected: five `OK` lines; the `comm` prints nothing.

- [ ] **Step 6: Commit**

```bash
git add docs/dashboards.md
git commit -m "docs(dashboards): document license, storage-pool and workload panels"
```

---

## Manual verification (not a task — reviewer/author note)

The metric cross-check guards against typo'd names, but panel *rendering* is only confirmed live. When a stacked local stack is available:

```bash
PSCALE1_PASSWORD='…' docker compose up -d --build
# Grafana http://localhost:3000 (admin/admin): all four PowerScale boards load,
# template vars populate, no "Datasource not found". license/storagepool panels show data;
# the Workloads board is empty until a performance dataset exists (expected).
```

---

## Self-Review

**1. Spec coverage:**
- §1a Overview Licensing row (status + days tables) → Task 2 (ids 27, 28). ✓
- §1b Capacity&SLA license stats (min days, expired) → Task 2 (ids 24, 25). ✓
- §2 Storage Pools row (table + used-% ts + SSD/HDD ts) → Task 1 (ids 20, 21, 22). ✓
- §3 Workloads board (5 vars, ops/throughput/cpu ts + snapshot table + prereq note) → Task 3. ✓
- §4 docs/dashboards.md → Task 4. ✓
- Validation (JSON-valid, metric cross-check, uid uniqueness, provisioning smoke) → each task + Task 4 + manual note. ✓
- Append-at-bottom placement, no reformatting, schemaVersion 39, tags, per-second-no-rate() → Global Constraints + every fragment. ✓

**2. Placeholder scan:** No TBD/TODO; every panel carries full JSON, exact `expr`, and a description. ✓

**3. Type/id/coordinate consistency:**
- capacity-sla ids 19–22 (Task 1) then 23–25 (Task 2) — no overlap, all > existing max 18. ✓
- overview ids 26–28 — > existing max 25. ✓
- workloads ids 1–6 (fresh file). ✓
- `y` layout: capacity-sla Task 1 occupies 46–64, Task 2 starts at 64 (row h1 at y64, stats at y65) — contiguous, no overlap. overview licensing at y54–63 (past the collapsed Per-Node Detail row at y53). ✓
- Every referenced metric is in the Global Constraints list and exists in `derivations.go` (enforced by the cross-check). Label names in `{…}` filters and `{{…}}` legends match the emitted label sets (`pool`/`type`; `zone`/`protocol`/`username`/`job_type`/`node`; `name`/`status`). ✓
- Append anchors: capacity-sla `    }\n  ],\n  "refresh": "1m",` (unique — only the top-level panels array precedes `"refresh"`); overview `    }\n  ]\n}` (unique — EOF). Each appended block ends in `    }`, so Task 2's capacity-sla append composes onto Task 1's. ✓
