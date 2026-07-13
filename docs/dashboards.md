# Dashboards

Three ready-made Grafana dashboards ship in the repo under
`grafana/provisioning/dashboards/json/`:

| Dashboard | uid | Focus |
| --- | --- | --- |
| **PowerScale / OneFS Overview** | `powerscale-overview` | Capacity, performance, protocols — day-to-day health. |
| **PowerScale / OneFS Advanced** | `powerscale-advanced` | Node/drive health, data protection, cache efficiency, quota & CPU detail. |
| **PowerScale / OneFS Capacity & SLA** | `powerscale-capacity-sla` | Availability/latency SLIs and capacity headroom with a days-to-full forecast. |

Because the `powerscale_` prefix matches
[`dell/csm-metrics-powerscale`](https://github.com/dell/csm-metrics-powerscale), existing
CSM dashboards also work against this exporter without modification.

Every panel carries a description (hover the ⓘ); provisional panels note that their stat keys are not yet live-validated against OneFS.

## Overview dashboard

One comprehensive board with these rows (in display order):

- **SLI Summary** — clusters up, per-cluster up/down status, detected OneFS API version,
  last-scrape age, and NFS export / SMB share / snapshot counts.
- **Capacity — Utilization & Saturation** — capacity used %, a used/available/free/total
  timeseries, and a table of the top quotas (usage vs hard limit).
- **Compute — Utilization** — cluster system / user / idle percent.
- **Network & Disk — Utilization** — external in/out throughput and cluster disk IOPS.
- **Protocol — Rate & Duration (RED)** — operations and average latency, broken down by
  `protocol` and `op`.
- **Per-Node Detail** *(collapsed by default)* — node CPU idle, memory used, disk IOPS, and
  used capacity.

### Template variables

| Variable | Source |
| --- | --- |
| `datasource` | Any Prometheus datasource — so the board works on import, not just in the bundled stack. |
| `cluster` | `label_values(powerscale_up, cluster)` — multi-select, includes *All*. |
| `node` | `label_values(powerscale_node_cpu_idle_percent{cluster=~"$cluster"}, node)` — drives the Per-Node Detail row. |

!!! note "Per-second gauges"
    IOPS / throughput / protocol panels read per-second gauges directly with `sum`/`avg`
    — they intentionally do **not** use `rate()`. See the [Metrics Reference](metrics.md).

## Advanced dashboard

The Advanced board surfaces the health/state and efficiency metrics, with a link back to
the Overview board.

### Validated rows

- **Cluster Health** — nodes read-only / smartfailing, drives by state, active events by
  severity, SyncIQ policies failed.
- **Data Protection** — SyncIQ policy table (enabled + last-run-failed) and snapshot space
  used.
- **Node CPU Detail** — per-node sys / user / idle.
- **Quota Detail** — logical + physical usage vs advisory / soft / hard thresholds.

### Provisional rows (collapsed by default)

The following rows are collapsed by default because their underlying stat keys or schemas
were validated against the OneFS 9.14.0 API specification but (except where noted) not yet
confirmed against a live cluster.

- **Cache Efficiency** — L1/L2/L3 read hit-vs-miss and a computed hit-ratio, from the
  node-scoped `node.ifs.cache.*` keys. Both the key names and their unit semantics
  (per-second bytes/second rates) are now confirmed against a live cluster via `--trace`
  (see [Metrics Reference](metrics.md#cache)); the row stays collapsed by default alongside
  the other advanced rows.
- **Storage Efficiency** — deduplication logical saved / deduplicated bytes *(provisional)*.
- **Per-Drive** — top drive IOPS and busy % *(provisional)*.
- **Per-Client** — operations by protocol/class and throughput in/out *(provisional)*.
- **Hardware** — power-supply failures, node temperature and fan speed *(provisional)*.

## Capacity & SLA dashboard

A focused planning board built **only on live-validated metrics** (no provisional keys),
in two sections:

- **SLA — Availability & Error Budget** — a stat strip (cluster availability %, scrape
  freshness, nodes read-only / smartfailing, SyncIQ failures, active critical events).
- **SLA — Latency, Throughput & Errors (RED)** — protocol latency (Duration), protocol
  operations (Rate), and active events by severity (Errors).
- **Capacity — Headroom & Forecast** — capacity used % gauge, a **Days to Full** stat and a
  **7-day forecast** line (both from `predict_linear`/`deriv` over the trailing 24h trend),
  per-node capacity balance, snapshot space, and a quota table ranked by closeness to the
  hard limit.

!!! note "Forecast is a trend projection"
    Days-to-full and the +7d line extrapolate the trailing 24h growth rate; a flat or
    shrinking trend reads as effectively "never". Use a longer dashboard time range
    (default `now-7d`) so the projection has history to draw from.

## Auto-provisioned (compose stack)

The [local stack](getting-started/quickstart.md) provisions both the Prometheus
datasource and this dashboard. `docker-compose.yml` mounts `./grafana/provisioning` into
Grafana, and `grafana/provisioning/dashboards/dashboards.yml` loads every JSON under
`json/`:

```bash
PSCALE1_PASSWORD='your-monitor-password' docker compose up -d --build
# Grafana: http://localhost:3000  (admin / admin)
# Dashboards → PowerScale / OneFS Overview
```

## Manual import (existing Grafana)

1. **Dashboards → New → Import**.
2. Upload `grafana/provisioning/dashboards/json/powerscale-overview.json` (or paste its
   contents).
3. Pick your Prometheus datasource when prompted, then **Import**.

## Customising

The provider sets `allowUiUpdates: true`, so you can tweak panels in the Grafana UI. To
persist changes, export the dashboard JSON (**Share → Export → Save to file**) and replace
the file in the repo. Keep the `uid` stable so links and provisioning stay consistent.

## Node Exporter Full (Grafana 1860)

This repo bundles the community [Node Exporter Full](https://grafana.com/grafana/dashboards/1860-node-exporter-full/)
dashboard (`node-exporter-full.json`, auto-provisioned). It visualizes **host OS** metrics
(CPU, memory, disk, network) exposed by [`prom/node-exporter`](https://hub.docker.com/r/prom/node-exporter) —
**not** this exporter's own metrics.

`node_exporter` is **not** part of this demo stack: it belongs on the hosts you actually want to
monitor, not bolted onto the exporter's compose. To use this dashboard, run `prom/node-exporter`
on those hosts and add a `node-exporter` scrape job to your Prometheus; the dashboard then
visualizes them.
