# Dashboards

Two ready-made Grafana dashboards ship in the repo under
`grafana/provisioning/dashboards/json/`:

| Dashboard | uid | Focus |
|---|---|---|
| **PowerScale / OneFS Overview** | `powerscale-overview` | Capacity, performance, protocols — day-to-day health. |
| **PowerScale / OneFS Advanced** | `powerscale-advanced` | Node/drive health, data protection, cache efficiency, quota & CPU detail. |

Because the `powerscale_` prefix matches
[`dell/csm-metrics-powerscale`](https://github.com/dell/csm-metrics-powerscale), existing
CSM dashboards also work against this exporter without modification.

## Overview dashboard

One comprehensive board with these rows:

- **Health & Overview** — clusters up, per-cluster up/down status, detected OneFS API
  version, last-scrape age, and NFS export / SMB share / snapshot counts.
- **Capacity & Quotas** — capacity used %, a used/available/total timeseries, and a table
  of the top quotas (usage vs hard limit).
- **CPU** — cluster system / user / idle percent.
- **Network & Disk** — external in/out throughput and cluster disk IOPS.
- **Protocol** — operations and average latency, broken down by `protocol` and `op`.
- **Per-Node** — node CPU idle, memory used, disk IOPS, and used capacity.

### Template variables

| Variable | Source |
|---|---|
| `datasource` | Any Prometheus datasource — so the board works on import, not just in the bundled stack. |
| `cluster` | `label_values(powerscale_up, cluster)` — multi-select, includes *All*. |
| `node` | `label_values(powerscale_node_cpu_idle_percent{cluster=~"$cluster"}, node)` — drives the Per-Node row. |

!!! note "Per-second gauges"
    IOPS / throughput / protocol panels read per-second gauges directly with `sum`/`avg`
    — they intentionally do **not** use `rate()`. See the [Metrics Reference](metrics.md).

## Advanced dashboard

The Advanced board surfaces the health/state and efficiency metrics, with a link back to
the Overview board:

- **Cluster Health** — nodes read-only / smartfailing, drives by state, active events by
  severity, SyncIQ policies failed.
- **Data Protection** — SyncIQ policy table (enabled + last-run-failed) and snapshot space
  used.
- **Cache Efficiency** — L1/L2/L3 read hit-vs-miss and a computed hit-ratio. These use the
  *provisional* `cache.*` keys; the panels stay empty if your OneFS release exposes
  different key strings (see [Metrics Reference](metrics.md#cache-provisional)).
- **Node CPU Detail** — per-node sys / user / idle.
- **Quota Detail** — usage vs advisory / soft / hard thresholds.
- **Storage Efficiency** — deduplication logical saved / deduplicated bytes *(provisional)*.
- **Per-Drive** — top drive IOPS and busy % *(provisional)*.
- **Per-Client** — operations by protocol/class and throughput in/out *(provisional)*.

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
