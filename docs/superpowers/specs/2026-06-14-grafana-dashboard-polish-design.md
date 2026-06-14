# Grafana Dashboard Polish — RED/USE Restructure + "Pro" Pass

Date: 2026-06-14
Status: Approved (design)
Scope: `grafana/provisioning/dashboards/json/powerscale-overview.json`,
`grafana/provisioning/dashboards/json/powerscale-advanced.json`, and a docs
refresh in `docs/dashboards.md`.

## Goal

Make the two bundled Grafana dashboards "crispy, pro, focused, and logical":

- **Logic** — reorganize panels around the RED (Rate/Errors/Duration) and USE
  (Utilization/Saturation/Errors) observability methods so the eye flows from
  cluster health → resource detail.
- **Focus** — demote per-node noise and unvalidated panels to collapsed rows so
  the default view answers one question per dashboard.
- **Pro / crispy** — add a description to every panel, give timeseries legends
  value calcs, and apply consistent threshold/color discipline.

## Non-goals / guardrails

- **No exporter code, metric names, queries, or label changes.** This is a
  layout + `fieldConfig` + metadata edit only. Every PromQL expression already
  present is preserved verbatim.
- No new datasource, no new template variables.
- Keep `schemaVersion: 39`, `refresh: 30s`, and the existing
  `datasource` / `cluster` / `node` template variables.
- Keep the `powerscale_` metric prefix and per-second gauge semantics (no
  `rate()` introduced).

## Shared conventions (both dashboards)

Applied uniformly to every panel:

1. **Description on every panel.** One or two sentences: what it shows + how to
   read it. Renders as the panel ⓘ tooltip. This is the largest "pro" lift —
   today only 3 of ~20 Overview panels have one.
2. **Timeseries legend** = `table` displayMode, placement `bottom`, calcs
   `["lastNotNull", "max", "mean"]`; tooltip mode `multi`, sort `desc`.
3. **Threshold + color discipline:**
   - Capacity % and CPU %: green `<70`, yellow `70`, orange `85`, red `95`.
   - Last Scrape Age (`s`): green `0`, red at `2×` the collection interval
     (default threshold `120`s for a 60s interval) — staleness is a trust signal.
   - Boolean health stats (readonly, smartfail, PSU failures, SyncIQ failed,
     critical events): green `0`, red `>0`.
4. **Unit hygiene (already correct, standardized):** `bytes`, `Bps`, `ops`,
   `percent`, `µs`. Standardize `decimals` (0 for counts, 1 for percent/latency)
   and clarify per-second meaning in panel titles where helpful (e.g.
   "Disk IOPS (ops/s)").
5. **Provisional caveat placement:** move the "keys not yet live-validated
   against OneFS" note out of row *titles* and into each affected panel's
   *description*; the row title becomes a clean name.

## Overview dashboard — "Is the cluster healthy *now*?"

USE-led, top-down. Row order:

1. **SLI Summary** (stat strip): Clusters Up · Cluster Status · Capacity Used %
   · Last Scrape Age (thresholded) · Active Critical Events · OneFS API Version
   · NFS Exports · SMB Shares · Snapshots.
   - Change vs today: **Active Critical Events** (`powerscale_active_events` with
     `severity="critical"`) is promoted onto this strip as a top-line SLI.
2. **Capacity — Utilization & Saturation**: Used % gauge · Cluster Capacity
   timeseries (used/available/total) · Top Quotas table (add a percent
   bar-gauge column for usage-vs-hard).
3. **Compute — Utilization**: Cluster CPU (sys / user / idle).
4. **Network & Disk — Utilization**: External Network Throughput · Disk IOPS.
5. **Protocol — Rate & Duration (RED)**: Protocol Operations (Rate) · Protocol
   Latency (Duration). (No protocol-level error metric exists; R + D only.)
6. **Per-Node detail** — **collapsed by default** (`row.collapsed: true` with its
   panels nested): Node CPU · Node Memory · Node Disk IOPS · Node Used Capacity.

## Advanced dashboard — "*Why / where* is the problem?"

USE/RED per resource. Row order:

1. **Cluster Health & Events**: Nodes Read-only · Nodes Smartfailing · Active
   Critical Events · SyncIQ Policies Failed · Drives-by-State table ·
   Active Events by Severity.
2. **Data Protection**: SyncIQ Policies table · Snapshot Space Used.
3. **Compute detail**: Node CPU (sys / user / idle).
4. **Quota detail**: Quota Usage vs Thresholds table.
5. **Cache Efficiency** — **collapsed, provisional** (description caveat).
6. **Storage Efficiency (dedupe)** — **collapsed, provisional**.
7. **Per-Drive** — **collapsed, provisional**.
8. **Per-Client** — **collapsed, provisional**.
9. **Hardware (PSU / temperature / fan)** — **collapsed, provisional**.

All five provisional rows use `row.collapsed: true` so they are hidden until
explicitly expanded, keeping the default view focused on validated panels.

## Collapsed-row mechanics

In schemaVersion 39, a collapsed row is a `type: "row"` panel with
`"collapsed": true` and the row's child panels nested inside its `panels: []`
array (not left as siblings). The restructure must move each collapsed row's
child panels into that array and set their `gridPos` relative to the row.
Validation must confirm panels render when the row is expanded.

## Validation

- `jq empty <file>` on both files (valid JSON).
- Panel/target count parity: no PromQL `expr` string is added, removed, or
  altered (diff the sorted set of `expr` values before/after — must be
  identical).
- `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
  still passes (docs reference the dashboards).
- Manual: import both JSON files into a Grafana instance, confirm no "panel
  plugin not found" / schema errors and that collapsed rows expand correctly.
- Semgrep write-hook passes on every edited file.

## Risks

- **Hand-editing 777-line JSON is error-prone.** Mitigation: edit
  programmatically where structural (row nesting) and validate with `jq` + an
  `expr`-set diff after every file write.
- **gridPos churn** when nesting collapsed rows can overlap panels. Mitigation:
  recompute `y` offsets per row and verify in a live Grafana import.
