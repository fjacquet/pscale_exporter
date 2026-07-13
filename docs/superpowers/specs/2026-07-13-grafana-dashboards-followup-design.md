# Design: Grafana dashboards for the three new collectors (license / storagepool / workload)

Date: 2026-07-13
Status: approved for planning
Context: follow-up to the three v0.13.0-review collectors — license (#34 / PR #35), storagepool (#33 / PR #36), workload (#32 / PR #37). Those PRs ship the metrics and `docs/metrics.md` reference but **no dashboard panels**. This closes the visualization gap.
Branch: `feat/dashboards-followup`, stacked on `feat/workload-metrics` (so all three metric families are present for a local provisioning smoke test); auto-retargets to `main` as the collector PRs merge.
Scope: `grafana/provisioning/dashboards/json/powerscale-overview.json`, `…/powerscale-capacity-sla.json`, a new `…/powerscale-workloads.json`, and `docs/dashboards.md`.

## Goal

Add Grafana panels for the three shipped-but-unvisualized metric families so operators can *see* license expiry, per-pool/tier capacity headroom, and per-workload performance — not just scrape them.

## Placement strategy — hybrid

Decided during brainstorming:
- **license** and **storagepool** fold into the existing boards where operators already look (Overview, Capacity & SLA).
- **workload** — high-cardinality, dataset-dependent, the odd one out — gets its own dedicated `powerscale-workloads.json`.

## Grounding facts (verified against the code, not assumed)

Exact metric names + labels (from `internal/powerscale/derivations.go` / `metrics.go`):

**License** (`baseLabels` = `cluster`, `cluster_id`):
- `powerscale_license_info{cluster,cluster_id,name,status}` = `1` (status carried as a label)
- `powerscale_license_expired{cluster,cluster_id,name}` = `0|1`
- `powerscale_license_days_to_expiry{cluster,cluster_id,name}` = days — **emitted only for licenses that carry an expiration** (perpetual omit it, so `min(...)` never false-fires a "<30 days" alert).

**Storage pools** (`{cluster,cluster_id,pool,type}`, `type` ∈ `nodepool|tier`), 9 gauges:
- aggregate: `powerscale_storagepool_{total,used,available}_capacity_bytes`
- media split: `powerscale_storagepool_{ssd,hdd}_{total,used,available}_capacity_bytes`
- **Double-count caveat:** a tier's capacity is the sum of its child node pools; summing across all rows double-counts. Consumers filter `type="nodepool"` for a non-overlapping cluster total.

**Workload** (`{cluster,cluster_id,node,zone,protocol,username,system_name,job_type}`), 4 per-second gauges:
- `powerscale_workload_operations_per_second`, `…_in_bytes_per_second`, `…_out_bytes_per_second`, `…_cpu_microseconds_per_second`
- All are per-second gauges (`sum`/`avg`, never `rate()`). Rows exist only when a OneFS **performance dataset** is configured; unpinned dimensions render as `""`.

Conventions (from the existing three boards): `schemaVersion 39`; `uid` = filename base; template-var trio `datasource` (type `prometheus`) / `cluster` (`label_values(powerscale_up, cluster)`) / `node`; panel types `row/stat/table/timeseries/gauge`; tags start `[powerscale, onefs, dell, …]`; per-second panels never use `rate()`. Provisioning (`dashboards.yml`) globs the whole `json/` folder, so a new board is auto-loaded with **no config edit**.

**Readiness:** per the brainstorming decision, new content ships **expanded / treated-as-ready** — no "provisional, not-yet-live-validated" caveat (a deliberate departure from the Cache/Per-Drive rows' collapsed-provisional convention). The workload prerequisite note below is functional (the board is empty without a dataset), not a provisional marker.

## 1. License → Overview board + Capacity & SLA board

### 1a. Overview board — new "Licensing" row (expanded), placed immediately after the *SLI Summary* row

- **Table · License status** — `powerscale_license_info{cluster=~"$cluster"}`, reduced to the label set; columns `cluster · name · status`. Value-mapping color overrides on `status`: `Licensed`/`Activated` → green, `Evaluation` → yellow, `Expired` → red, else neutral.
- **Table · Days to expiry** (sorted ascending) — `powerscale_license_days_to_expiry{cluster=~"$cluster"}`; columns `cluster · name · days`; thresholds red `<30`, yellow `<90`, green `≥90`. Only expiring licenses appear.

### 1b. Capacity & SLA board — two stats appended to the existing *SLA — Availability & Error Budget* row

- **Stat · Min days to license expiry** — `min(powerscale_license_days_to_expiry{cluster=~"$cluster"})`; thresholds red `<30` / yellow `<90` / green `≥90`; *No data* when nothing expires.
- **Stat · Licenses expired** — `sum(powerscale_license_expired{cluster=~"$cluster"})`; `0` green, `≥1` red.

## 2. Storage pools → Capacity & SLA board

New **"Storage Pools — Capacity"** row inside the *Capacity — Headroom & Forecast* section (expanded). Row/panel descriptions carry the double-count caveat.

- **Table · Pool capacity** — one row per pool/tier, joined instant queries:
  - `used%` = `100 * powerscale_storagepool_used_capacity_bytes{cluster=~"$cluster"} / powerscale_storagepool_total_capacity_bytes{cluster=~"$cluster"}`
  - `powerscale_storagepool_{used,total,available}_capacity_bytes{cluster=~"$cluster"}`
  - `powerscale_storagepool_{ssd,hdd}_{used,total}_capacity_bytes{cluster=~"$cluster"}`
  - Columns: `pool · type · used% · used · total · avail · SSD used/total · HDD used/total`. Byte unit on capacity columns; `used%` thresholds yellow `75`, red `90`.
- **Timeseries · Pool used %** — `100 * powerscale_storagepool_used_capacity_bytes{cluster=~"$cluster",type="nodepool"} / powerscale_storagepool_total_capacity_bytes{cluster=~"$cluster",type="nodepool"}` by `pool` (nodepool-only, so tiers don't overlap their children). `percent` unit, 0–100.
- **Timeseries · SSD vs HDD available** — `powerscale_storagepool_ssd_available_capacity_bytes{cluster=~"$cluster"}` and `…hdd_available…` by `pool` (legend `{pool} SSD` / `{pool} HDD`). `bytes` unit — surfaces the media headroom the 6 media gauges exist for.

## 3. Workload → new `powerscale-workloads.json`

- `uid: powerscale-workloads`; title `PowerScale / OneFS Workloads`; tags `[powerscale, onefs, dell, workload, performance]`; `schemaVersion 39`; link back to the Overview board (matching how Advanced links back).
- **Dashboard description + one compact text panel** state the prerequisite: *requires a configured OneFS performance dataset (`isi performance datasets`); the board is empty otherwise.* Functional, not a provisional marker.
- **Template variables** (the safeguard that makes an uncapped full-series view usable):
  - `datasource` — type `prometheus`
  - `cluster` — `label_values(powerscale_up, cluster)`, multi-select + *All*
  - `zone` — `label_values(powerscale_workload_operations_per_second{cluster=~"$cluster"}, zone)`, multi-select + *All*
  - `protocol` — `label_values(powerscale_workload_operations_per_second{cluster=~"$cluster"}, protocol)`, multi-select + *All*
  - `username` — `label_values(powerscale_workload_operations_per_second{cluster=~"$cluster"}, username)`, multi-select + *All*
- **One "Per-workload performance" row (expanded)**, every query filtered by `{cluster=~"$cluster",zone=~"$zone",protocol=~"$protocol",username=~"$username"}`, legend rendered as a table (last + max):
  - **Timeseries · Operations/s per workload** — `powerscale_workload_operations_per_second{…}`, legend `{{zone}}·{{protocol}}·{{username}}·{{job_type}}·node{{node}}`. Unit `ops`.
  - **Timeseries · Throughput per workload** — `…_in_bytes_per_second{…}` + `…_out_bytes_per_second{…}`. Unit `Bps`.
  - **Timeseries · CPU µs/s per workload** — `…_cpu_microseconds_per_second{…}`. Unit `µs` (per-second rate; documented, not `rate()`-d).
  - **Table · Workload snapshot** (instant) — all six dims + the four values; sortable, default sort by ops desc.

**Design tension (accepted):** full per-workload timeseries + expanded/ready means the default *All* view can render many series on a broad dataset. The template-var filters are the mitigation; no `topk` cap is imposed (deliberate — matches the "full per-workload timeseries" decision). Documented in `docs/dashboards.md` so operators know to slice with the variables.

## 4. Docs — `docs/dashboards.md`

- Add the **Workloads** board to the dashboard table (uid `powerscale-workloads`, focus "per-workload performance; requires a performance dataset").
- Document the new **Licensing** row (Overview) and the **Storage Pools — Capacity** row + the two license SLA stats (Capacity & SLA).
- Add a subsection for the Workloads board: the four panels, the template-var filters, the performance-dataset prerequisite, and the "slice with the variables to bound series" guidance.

## Testing / validation

Dashboards are JSON with no Go bindings, so there are no unit tests — and the repo has no existing dashboard CI. Validation:
1. **Valid JSON** — `python3 -m json.tool` (or `jq empty`) on each edited/new file; must parse.
2. **Metric-name cross-check** — every `powerscale_*` metric referenced in the new/edited JSON must exist in `internal/powerscale/derivations.go` (grep both sides; no typos, no stale names). This is the key correctness gate — a mistyped metric silently yields an empty panel.
3. **uid uniqueness** — `powerscale-workloads` must not collide with the existing four uids.
4. **Provisioning smoke (manual)** — `PSCALE1_PASSWORD=… docker compose up -d --build`; confirm all four PowerScale boards load, template variables populate, and no "Datasource not found" — license/storagepool panels show data from the stacked collectors (workload stays empty without a dataset, as expected).

## Non-goals

- **No `topk`/cardinality cap** on the workload board — the "full per-workload timeseries" decision; template-var filters are the bound instead.
- **No collapsed/provisional treatment** — the "expanded / treated-as-ready" decision.
- **No new template var** on the existing boards beyond the two license stats and one storagepool row (those reuse the existing `datasource`/`cluster` vars).
- **No alerting rules** — dashboards only; Prometheus alert rules for expiry/headroom are a separate future item.
- **No changes to `powerscale-advanced.json`** — workload lands on its own board, not Advanced.

## Risks

- **Workload series volume** — mitigated by template-var filters; accepted per the design decision, documented for operators.
- **Live-unverified metrics** — like the collectors themselves, panels are built from schema-validated metrics not yet live-confirmed; a value-scale surprise (e.g. cpu µs/s) would need a threshold/unit tweak, not a structural change. Best-effort collectors mean a missing metric yields an empty panel, never a broken board.
- **Manual smoke only** — no automated dashboard test in CI; the metric-name cross-check is the automatable guard against the most likely defect (a typo'd metric name).
