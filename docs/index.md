# PowerScale Exporter

A Go exporter for **Dell PowerScale (OneFS)**. It authenticates to the OneFS platform
API, collects broad cluster, node, protocol, quota, snapshot and capacity metrics, and
exposes them via **both** a Prometheus `/metrics` endpoint **and** an optional OTLP
metric push — both fed from one shared snapshot.

The `powerscale_` metric prefix matches
[`dell/csm-metrics-powerscale`](https://github.com/dell/csm-metrics-powerscale), so
existing CSM Grafana dashboards work without modification.

## Features

- **Dual export** — Prometheus pull (`/metrics`) and an optional OTLP metric push, fed
  from one shared snapshot.
- **Broad OneFS coverage** — cluster, nodes, protocols (NFS/SMB export & share counts),
  quotas, snapshots, and capacity. Combines typed OneFS resources with the raw statistics
  API (curated stat keys plus the protocol summary).
- **Multi-cluster** — one process monitors many OneFS clusters; every metric carries a
  `cluster` label.
- **Operational** — one shared gopowerscale session per cluster, graceful per-cluster
  degradation (an unreachable cluster is marked down without taking the exporter down),
  hot config reload (SIGHUP + file watch), snapshot-based `/health`, and optional OTLP
  tracing.
- **CSM-compatible naming** — the `powerscale_` prefix matches Dell's
  `csm-metrics-powerscale` so existing dashboards work.

## Where to next

- **Getting started** — [Installation](getting-started/installation.md) ·
  [Configuration](getting-started/configuration.md) ·
  [Quick Start](getting-started/quickstart.md)
- **[Metrics Reference](metrics.md)** — every exported series, its labels and units.
- **Deployment** — [Docker](deployment/docker.md) · [systemd](deployment/systemd.md) ·
  [Kubernetes](deployment/kubernetes.md)
- **[Dashboards](dashboards.md)** — the bundled Grafana dashboard.
- **[OpenTelemetry](opentelemetry.md)** — the OTLP metric/trace push path.
- **[CI/CD & SBOM](cicd.md)** — build, release, and supply-chain artifacts.

## How it works

A single background collection loop polls every configured cluster on
`collection.interval` and publishes an immutable **snapshot**. Both export paths read the
latest snapshot rather than fetching on scrape, which decouples OneFS API load from the
number of scrapers and the OTLP push cadence.

!!! note "Per-second gauges"
    `iops` and bandwidth metrics are already **per-second gauges** — aggregate them with
    `sum`/`avg` in PromQL, **never** `rate()`.
