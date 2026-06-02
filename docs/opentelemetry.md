# OpenTelemetry

Besides the Prometheus `/metrics` pull endpoint, the exporter can **push** metrics (and
optionally traces) over OTLP/gRPC. Both export paths read the same shared snapshot, so the
push cadence is independent of the collection interval and of any Prometheus scrapers.

## Configuration

```yaml
opentelemetry:
  metrics:
    enabled: true
    endpoint: "otel-collector:4317"
    insecure: true
    interval: "30s"
  tracing:
    enabled: false
    endpoint: "otel-collector:4317"
    insecure: true
    samplingRate: 0.1
```

| Field | Meaning |
|---|---|
| `metrics.enabled` | Turn on the OTLP metric push. |
| `metrics.endpoint` | OTLP/gRPC collector address (`host:port`). |
| `metrics.insecure` | `true` for plaintext gRPC (lab/in-cluster); use TLS otherwise. |
| `metrics.interval` | How often metrics are pushed. |
| `tracing.enabled` | Emit OTLP traces for the collection loop. |
| `tracing.samplingRate` | Fraction of traces sampled (0.0–1.0). |

The metric push uses **asynchronous observable gauges** driven by a periodic reader that
reads the latest snapshot — the same data served on `/metrics`, so series names and labels
match the [Metrics Reference](metrics.md).

## Collector pipeline

The bundled `otel-collector-config.yaml` receives OTLP on `:4317` and re-exposes the
metrics on `:8889` for Prometheus to scrape:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
processors:
  batch: {}
exporters:
  debug:
    verbosity: normal
  prometheus:
    endpoint: 0.0.0.0:8889
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug, prometheus]
```

## End-to-end in the compose stack

The [local stack](getting-started/quickstart.md) wires this up: enable
`opentelemetry.metrics` pointing at `otel-collector:4317`, recreate the exporter, and
Prometheus picks up the re-exposed series via its `powerscale-otlp` job
(`otel-collector:8889`). This lets you observe **both** export paths side by side —
`powerscale` (direct pull) and `powerscale-otlp` (push → collector → scrape).
