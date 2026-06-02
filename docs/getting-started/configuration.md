# Configuration

The exporter reads a single YAML file (default `config.yaml`, override with `--config`).
One process monitors many OneFS clusters.

## Command-line flags

| Flag | Purpose |
|---|---|
| `--config <path>` | Path to the config file (default `config.yaml`). |
| `--debug` | Verbose logging. |
| `--once` | Run a single collection cycle and exit (useful for smoke tests / cron). |

## Full example

```yaml
server:
  host: "0.0.0.0"
  port: "2112"
  uri: "/metrics"
  logName: "/var/log/pscale_exporter/pscale-exporter.log"

collection:
  interval: "30s"   # OneFS perf samples are ~30s native; capacity changes slowly
  timeout: "20s"

opentelemetry:
  metrics:
    enabled: false
    endpoint: "localhost:4317"
    insecure: true
    interval: "30s"
  tracing:
    enabled: false
    endpoint: "localhost:4317"
    insecure: true
    samplingRate: 0.1

# A read-only account (role with ISI_PRIV_STATISTICS + ISI_PRIV_QUOTA) is sufficient.
clusters:
  - name: pscale-cluster1
    endpoint: pscale-clu1.example.com
    port: 8080
    username: pscale-monitor
    password: "${PSCALE1_PASSWORD}"
    # Enable ONLY for clusters with self-signed certs (common in test/lab); keep false in production.
    insecureSkipVerify: false
```

## Blocks

### `server`

The HTTP listener. `uri` is the metrics path (`/metrics`); `/health` is always served and
is snapshot-based. `logName` is the log file path — make sure the runtime user can write
it (the container image pre-creates `/var/log/pscale_exporter` owned by `uid 10001`).

### `collection`

- `interval` — how often the background loop polls every cluster and publishes a new
  snapshot. OneFS performance samples are natively ~30s, so going below that buys little.
- `timeout` — per-collection deadline for a cluster.

Both export paths read the **latest snapshot** rather than fetching on scrape, so the
scrape/push cadence is independent of OneFS API load.

### `opentelemetry`

Optional OTLP push for metrics and/or tracing. Disabled by default. See
[OpenTelemetry](../opentelemetry.md) for the full push path.

### `clusters`

A list — one entry per OneFS cluster. Every metric carries a `cluster` label set to
`name`. `insecureSkipVerify` should stay `false` in production; enable it only for lab
clusters with self-signed certificates.

## Secrets

Never put plaintext passwords in the file. Two options:

- **Environment reference** — `password: "${PSCALE1_PASSWORD}"` expands the named env var
  at load time. Export it before starting the process (or pass `-e` to `docker run`).
- **`passwordFile`** — point at a file whose contents are the password (handy with
  Kubernetes/Docker secrets mounted as files).

## Hot reload

The exporter reloads its config without a restart on **`SIGHUP`** and on **file change**
(it watches the config file). Edit `config.yaml`, and the next collection cycle picks up
the new clusters/intervals.
