# pscale_exporter

[![CI](https://github.com/fjacquet/pscale_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/pscale_exporter/actions/workflows/ci.yml)
[![Release](https://github.com/fjacquet/pscale_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/fjacquet/pscale_exporter/actions/workflows/release.yml)
[![Docs](https://github.com/fjacquet/pscale_exporter/actions/workflows/docs.yml/badge.svg)](https://fjacquet.github.io/pscale_exporter/)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/pscale_exporter)](https://goreportcard.com/report/github.com/fjacquet/pscale_exporter)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fjacquet/pscale_exporter)](go.mod)
[![Latest Release](https://img.shields.io/github/v/release/fjacquet/pscale_exporter?sort=semver)](https://github.com/fjacquet/pscale_exporter/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A Go exporter for **Dell PowerScale (OneFS)**. It authenticates to the OneFS platform
API, collects broad cluster, node, protocol, quota, snapshot and capacity metrics, and
exposes them via **both** a Prometheus `/metrics` endpoint **and** an optional OTLP
metric push — both fed from one shared snapshot. The `powerscale_` metric prefix matches
[`dell/csm-metrics-powerscale`](https://github.com/dell/csm-metrics-powerscale) for
dashboard compatibility.

## Features

- **Dual export**: Prometheus pull (`/metrics`) and an optional OTLP metric push, fed
  from one shared snapshot.
- **Broad OneFS coverage**: cluster, nodes, protocols (NFS/SMB export & share
  counts), quotas, snapshots, and capacity — combining typed OneFS resources with the
  raw statistics API (curated stat keys plus the protocol summary).
- **Multi-cluster**: one process monitors many OneFS clusters; every metric carries a
  `cluster` label.
- **Operational**: one shared gopowerscale session per cluster, graceful per-cluster
  degradation (an unreachable cluster is marked down without taking the exporter down),
  hot config reload (SIGHUP + file watch), snapshot-based `/health`, and optional OTLP
  tracing.
- **CSM-compatible naming**: the `powerscale_` metric prefix matches Dell's
  `csm-metrics-powerscale` so existing dashboards work.

## Quick start

```bash
make cli
export PSCALE1_PASSWORD='your-monitor-password'
./bin/pscale_exporter --config config.yaml
# metrics: http://localhost:2112/metrics   health: http://localhost:2112/health
```

Or with Docker: `docker pull ghcr.io/fjacquet/pscale_exporter:latest`.

## Local test stack

A `docker compose` stack brings up the exporter alongside Prometheus, an OpenTelemetry
collector, and Grafana for a complete end-to-end test:

```bash
PSCALE1_PASSWORD='your-monitor-password' docker compose up -d --build
```

| Service | URL | Purpose |
|---|---|---|
| Exporter | <http://localhost:2112/metrics> (`/health`) | the `/metrics` pull endpoint |
| Prometheus | <http://localhost:9090> | scrapes the exporter; alert rules in `deploy/prometheus/pscale.rules.yml` |
| Grafana | <http://localhost:3000> (`admin`/`admin`) | Prometheus datasource auto-provisioned |
| OTLP collector | <http://localhost:8889/metrics> | receives the OTLP push and re-exposes it |

Point `config.yaml` at a real cluster and set `PSCALE1_PASSWORD` for a true end-to-end
run; with the default example cluster the stack still validates wiring (`powerscale_up`
goes to 0 and the `PowerScaleClusterDown` alert fires). To exercise the OTLP push path,
set `opentelemetry.metrics.enabled: true` with `endpoint: "otel-collector:4317"` in
`config.yaml`. A pull-based variant (published GHCR image, no local build) is in
`docker-compose.ghcr.yml`.

## Documentation

Full docs at **<https://fjacquet.github.io/pscale_exporter/>**:

- [Installation](https://fjacquet.github.io/pscale_exporter/getting-started/installation/) ·
  [Configuration](https://fjacquet.github.io/pscale_exporter/getting-started/configuration/) ·
  [Quick Start](https://fjacquet.github.io/pscale_exporter/getting-started/quickstart/)
- [Metrics Reference](https://fjacquet.github.io/pscale_exporter/metrics/)
- Deployment:
  [Docker](https://fjacquet.github.io/pscale_exporter/deployment/docker/) ·
  [systemd](https://fjacquet.github.io/pscale_exporter/deployment/systemd/) ·
  [Kubernetes](https://fjacquet.github.io/pscale_exporter/deployment/kubernetes/)
- [Dashboards](https://fjacquet.github.io/pscale_exporter/dashboards/) ·
  [OpenTelemetry](https://fjacquet.github.io/pscale_exporter/opentelemetry/) ·
  [CI/CD & SBOM](https://fjacquet.github.io/pscale_exporter/cicd/)

## Development

```bash
make tools         # install golangci-lint, cyclonedx-gomod, govulncheck (pinned)
make sure          # fmt + vet + test + build + golangci-lint
make ci            # the gate CI runs (adds go test -race + govulncheck)
```

## Notes

- A read-only OneFS account (a role with `ISI_PRIV_STATISTICS` + `ISI_PRIV_QUOTA`) is
  sufficient for collection.
- IOPS and bandwidth are already per-second gauges — aggregate with `sum`/`avg` in
  PromQL, never `rate()`.
- Metric names are unit-explicit: `_bytes`, `_bytes_per_second`,
  `_operations_per_second`, `_microseconds`, `_percent`.
- The `powerscale_` prefix matches Dell's `csm-metrics-powerscale` for dashboard reuse.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

