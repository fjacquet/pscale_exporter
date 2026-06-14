# ADR-0004: `powerscale_` metric prefix and unit-explicit naming

- **Status:** Accepted
- **Date:** 2026-06-14
- **Deciders:** Frederic Jacquet

## Context

Metric names are a public contract: once dashboards, alerts, and recording rules reference
them, renaming is a breaking change. Two naming decisions needed to be fixed early and held:

1. **The namespace prefix.** Dell ships an official metrics stack,
   `dell/csm-metrics-powerscale`, with published Grafana dashboards that expect a particular
   metric namespace. An exporter that invents its own prefix forces users to rewrite or
   duplicate those dashboards.
2. **Units and aggregation semantics.** PowerScale exposes rates, byte counts, latencies, and
   percentages. If units live only in `# HELP` text (or nowhere), consumers guess — and guess
   wrong, e.g. applying `rate()` to a value that is already per-second.

## Decision

**Use the `powerscale_` prefix for every exported metric, and encode the unit in the metric
name. Both are treated as a stable, non-negotiable contract.**

- **`powerscale_` prefix** — matches `dell/csm-metrics-powerscale` so existing dashboards work
  against this exporter with little or no change. Keep it.
- **Unit-explicit names** — every name carries its unit suffix: `_bytes`,
  `_bytes_per_second`, `_operations_per_second`, `_microseconds`, `_percent`. The unit is part
  of the name, not just the help text.
- **Per-second gauges are already rates.** `iops` and bandwidth metrics are emitted as
  per-second **gauges**; in PromQL they are aggregated with `sum`/`avg`, **never** `rate()`.
  This follows from OneFS reporting these as `*.rate` keys, which the exporter surfaces
  verbatim as gauges.
- **Canonical leading labels.** Every sample carries `cluster` and `cluster_id` as its first
  labels (node-scope samples add the node identifier), so dashboards can template on cluster
  uniformly.

These rules are enforced by convention in `statisticsKeys.json` (the `metric` column) and the
sample builders in `derivations.go`; they are documented for contributors in `CLAUDE.md` and
the metric reference in `docs/metrics.md`.

### Alternatives considered

- **A project-specific prefix (e.g. `pscale_` or `onefs_`).** Rejected: breaks compatibility
  with the published csm-metrics-powerscale dashboards, which is the main reason an operator
  would pick a drop-in exporter.
- **Units in `# HELP` only, unitless names.** Rejected: help text is invisible in most query
  and alert contexts; unit-in-name is the Prometheus best-practice and prevents the
  `rate()`-on-a-rate class of mistakes.
- **Emitting bandwidth/IOPS as counters.** Rejected: OneFS already reports these as rates;
  re-deriving counters would invent state the source doesn't provide and invite double-rating.

## Consequences

**Positive**

- Existing `dell/csm-metrics-powerscale` dashboards work against this exporter.
- Units are unambiguous at query time; the per-second-gauge rule is explicit, heading off
  `rate()` misuse.
- Uniform `cluster` / `cluster_id` labelling makes multi-cluster dashboards straightforward.

**Negative / costs**

- The prefix and the unit suffixes are now a frozen contract: renaming any metric is a
  breaking change for downstream dashboards and alerts, even when the underlying edit (a row
  in `statisticsKeys.json`) looks trivial. See [ADR-0003](0003-stat-key-table-coverage-contract.md).
- Tracking an upstream rename in csm-metrics-powerscale would force a coordinated, breaking
  bump here.
- Gauge-based rates mean historical reaggregation over long windows uses `avg_over_time`, not
  the counter-oriented `rate()`/`increase()` that some operators reach for by habit.

## References

- `dell/csm-metrics-powerscale` — the dashboard namespace this prefix matches
- `internal/powerscale/statisticsKeys.json` — `metric` column carries prefix + unit
- `docs/metrics.md` — the exported metric reference
- Prometheus naming best practices — base units, unit suffixes, rate vs gauge
- [ADR-0003](0003-stat-key-table-coverage-contract.md) — where metric names are declared
