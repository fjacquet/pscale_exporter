# ADR-0002: Snapshot collection model and dual (Prometheus + OTLP) export

- **Status:** Accepted
- **Date:** 2026-06-14
- **Deciders:** Frederic Jacquet

## Context

The exporter must expose Dell PowerScale (OneFS) metrics to two consumers at once: a
Prometheus `/metrics` pull endpoint and an optional OTLP metric push. Several forces shape
how collection and export should be wired:

1. **OneFS API load must not scale with the number of consumers.** The naive Prometheus
   pattern — collect on every scrape — would hit the OneFS platform API once per scraper per
   interval, and again on the OTLP cadence. With multiple Prometheus replicas plus an OTLP
   push, a single cluster could see a multiple of the intended request rate. OneFS is a
   storage controller, not a metrics backend; its statistics API should be polled at a
   bounded, predictable rate regardless of how many things read the result.
2. **The two export paths have different execution models.** Prometheus is *synchronous and
   pull-driven* (the registry calls `Collect` on each scrape). OTLP is *asynchronous and
   push-driven* (a periodic reader fires instrument callbacks on its own cadence). They must
   nonetheless emit the *same* numbers from the *same* collection cycle.
3. **The Prometheus metric-name set is dynamic.** Coverage is driven by a curated stat-key
   table (`statisticsKeys.json`); which metric names appear depends on what OneFS returns and
   what is configured, not on a compile-time list of descriptors.

## Decision

**Adopt a snapshot model: a single background collection loop owns all OneFS polling and
publishes an immutable snapshot; both export paths read the snapshot rather than fetching on
demand.**

- **One collection loop** (`collector.go`) polls every configured cluster on
  `collection.interval`, assembles a per-cluster result, and publishes a `*Snapshot`.
- **Immutable snapshot, pointer-swap store** (`snapshot.go`). `BuildSnapshot` produces a
  read-only value with a `byName` index of samples; `SnapshotStore` swaps the current pointer
  under an `RWMutex` so the loop can publish while exporters read concurrently. Readers never
  block each other and never see a half-updated snapshot.
- **`PromCollector` reads the snapshot** (`prometheus.go`). It is registered as an
  **unchecked collector**: `Describe` sends nothing, so it may emit a dynamic set of metric
  names by building each `prometheus.Desc` on the fly in `Collect`. Per-cluster health
  metrics (`powerscale_up`, `powerscale_last_scrape_timestamp_seconds`,
  `powerscale_cluster_api_version`) keep fixed descriptors. Duplicate label-tuples within a
  metric name are skipped to avoid registry gather errors.
- **`OTLPExporter` reads the same snapshot** (`otlp.go`) via asynchronous observable gauges
  driven by a periodic reader; each instrument callback reads the latest snapshot and
  observes its samples. Because the metric-name set is fixed by the statistic set, OTLP
  instruments are registered once.
- **`/health` is snapshot-based** (`main.go`): liveness/readiness is derived from the latest
  published snapshot, not from a live OneFS call.

This decouples OneFS API load from the number of scrapers and the OTLP push cadence: OneFS is
polled exactly once per `collection.interval`, and every consumer reads whatever was last
published.

### Alternatives considered

- **Collect on scrape (the default Prometheus pattern).** Rejected: ties OneFS request rate
  to scraper count, makes the OTLP path a second independent source of load, and risks the
  two paths reporting different values from different fetch instants.
- **A checked Prometheus collector with pre-registered descriptors.** Rejected: the
  metric-name set is data-driven via `statisticsKeys.json`; pre-declaring every descriptor
  would require code changes for each new curated key and couples the collector to the table.
- **Two independent collectors (one per export path).** Rejected: doubles OneFS load and
  invites drift between what Prometheus and OTLP report. A single shared snapshot guarantees
  both paths emit the same cycle's data.
- **A single export path (Prometheus only, or OTLP only).** Rejected: the deployment targets
  require both a scrape endpoint and a push channel; the snapshot makes supporting both nearly
  free since they are just two readers of one value.

## Consequences

**Positive**

- OneFS API load is constant in the number of consumers — bounded by `collection.interval`.
- Prometheus and OTLP always report the same cycle's numbers; no per-path drift.
- New curated stat keys need only a `statisticsKeys.json` row — the unchecked collector emits
  them with no code change.
- Concurrent scrapes and OTLP pushes are lock-cheap (RWMutex read + pointer read); the
  collection loop never blocks readers.
- `/health` is fast and cannot stall on a slow OneFS call.

**Negative / costs**

- **Metrics are as fresh as the last cycle.** A scrape returns data up to
  `collection.interval` old, not live. This is the deliberate trade for bounded API load;
  set the interval accordingly.
- The unchecked collector gives up Prometheus' descriptor-consistency checks; the collector
  must itself guard against duplicate label-tuples (it does).
- A failed collection leaves the prior snapshot in place until the next cycle; per-cluster
  `Up` / `ScrapeError` on the snapshot (and `powerscale_up`) signal staleness rather than the
  endpoint failing. See the graceful-degradation behaviour in `collector.go`.

## References

- `internal/powerscale/snapshot.go` — immutable snapshot + RWMutex pointer-swap store
- `internal/powerscale/prometheus.go` — unchecked collector reading the snapshot
- `internal/powerscale/otlp.go` — observable gauges driven by a periodic reader
- `internal/powerscale/collector.go` — the single collection loop and graceful degradation
- Prometheus client_golang: "unchecked collectors" (Collector with empty `Describe`)
