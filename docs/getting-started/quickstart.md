# Quick Start

## Run the exporter

```bash
make cli
export PSCALE1_PASSWORD='your-monitor-password'
./bin/pscale_exporter --config config.yaml
# metrics: http://localhost:2112/metrics   health: http://localhost:2112/health
```

Or with Docker:

```bash
docker run --rm -p 2112:2112 \
  -v "$PWD/config.yaml:/etc/pscale_exporter/config.yaml:ro" \
  -e PSCALE1_PASSWORD='your-monitor-password' \
  ghcr.io/fjacquet/pscale_exporter:latest
```

## Local end-to-end stack

A `docker compose` stack brings up the exporter alongside Prometheus, an OpenTelemetry
collector, and Grafana (with the [PowerScale dashboard](../dashboards.md) and Prometheus
datasource auto-provisioned) for a complete end-to-end test:

```bash
PSCALE1_PASSWORD='your-monitor-password' docker compose up -d --build
```

| Service | URL | Purpose |
|---|---|---|
| Exporter | <http://localhost:2112/metrics> (`/health`) | the `/metrics` pull endpoint |
| Prometheus | <http://localhost:9090> | scrapes the exporter; alert rules in `deploy/prometheus/pscale.rules.yml` |
| Grafana | <http://localhost:3000> (`admin`/`admin`) | Prometheus datasource + PowerScale dashboard auto-provisioned |
| OTLP collector | <http://localhost:8889/metrics> | receives the OTLP push and re-exposes it |

Point `config.yaml` at a real cluster and set `PSCALE1_PASSWORD` for a true end-to-end
run.

!!! tip "Validating the wiring without a real cluster"
    With the default example cluster (unreachable), the stack still validates wiring:
    `powerscale_up` goes to `0` and the `PowerScaleClusterDown` alert fires after 5m.

To exercise the OTLP push path, set `opentelemetry.metrics.enabled: true` with
`endpoint: "otel-collector:4317"` (and `insecure: true`) in `config.yaml`, then recreate
the exporter. Prometheus scrapes the collector's re-exposed series via the
`powerscale-otlp` job. A pull-based variant (published GHCR image, no local build) is in
`docker-compose.ghcr.yml`.

## Next steps

- Explore the [Metrics Reference](../metrics.md).
- Wire up the [Grafana dashboard](../dashboards.md).
- Deploy for real: [Docker](../deployment/docker.md) · [systemd](../deployment/systemd.md)
  · [Kubernetes](../deployment/kubernetes.md).
