# Docker

## Image

The published image runs as a non-root user (`uid 10001`), exposes port `2112`, and
defaults to `--config /etc/pscale_exporter/config.yaml`:

```bash
docker run --rm \
  -p 2112:2112 \
  -v "$PWD/config.yaml:/etc/pscale_exporter/config.yaml:ro" \
  -e PSCALE1_PASSWORD='your-monitor-password' \
  ghcr.io/fjacquet/pscale_exporter:latest
```

Mount a writable volume at `/var/log/pscale_exporter` if you keep file logging
(`server.logName`) enabled; the image pre-creates that directory owned by the runtime
user.

## Compose stacks

Two compose files are provided:

| File | Use |
|---|---|
| `docker-compose.yml` | Local end-to-end stack — **builds** the exporter image from source alongside Prometheus, an OTLP collector, and Grafana. |
| `docker-compose.ghcr.yml` | Pull-based variant using the published GHCR image (no local build). |

Bring up the full stack:

```bash
PSCALE1_PASSWORD='your-monitor-password' docker compose up -d --build
```

| Service | URL |
|---|---|
| Exporter | <http://localhost:2112/metrics> (`/health`) |
| Prometheus | <http://localhost:9090> |
| Grafana | <http://localhost:3000> (`admin`/`admin`) |
| OTLP collector | <http://localhost:8889/metrics> |

Grafana auto-provisions the Prometheus datasource **and** the
[PowerScale dashboard](../dashboards.md) from `./grafana/provisioning`. Override
`GF_ADMIN_USER` / `GF_ADMIN_PASSWORD` for anything non-local.

## Prometheus scrape & alerts

The bundled `prometheus.yml` defines two jobs — `powerscale` (scrapes the exporter
directly) and `powerscale-otlp` (scrapes the collector's re-exposed series). Alert rules
live in `deploy/prometheus/pscale.rules.yml`:

| Alert | Condition |
|---|---|
| `PowerScaleClusterDown` | `powerscale_up == 0` for 5m. |
| `PowerScaleExporterDown` | `up{job="powerscale"} == 0` for 5m. |
| `PowerScaleClusterCapacityHigh` | used/total capacity > 85% for 15m. |
| `PowerScaleQuotaNearHardLimit` | quota usage/hard > 90% for 15m. |
| `PowerScaleNodeSmartfail` | a node is smartfailing for 5m. |
| `PowerScaleNodeReadOnly` | a node is read-only for 10m. |
| `PowerScaleDriveUnhealthy` | any drive in SMARTFAIL/DEAD/STALLED/ERASE/GONE for 5m. |
| `PowerScaleSyncIQFailed` | a SyncIQ policy's last run failed for 15m (critical). |
| `PowerScaleActiveCriticalEvents` | unresolved critical OneFS events for 5m (critical). |

A minimal standalone scrape config:

```yaml
scrape_configs:
  - job_name: powerscale
    static_configs:
      - targets: ["pscale-exporter-host:2112"]
```
