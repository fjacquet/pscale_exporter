# ADR-0003: Curated stat-key table as the metric-coverage contract

- **Status:** Accepted
- **Date:** 2026-06-14
- **Deciders:** Frederic Jacquet

## Context

OneFS exposes a large, version-dependent set of statistics keys via
`/platform/1/statistics/current` (plus the per-protocol summary). The exporter must turn a
subset of those raw keys into stable, well-named Prometheus/OTLP metrics. Two questions drive
the design:

1. **How is coverage defined and extended?** New keys are added over time as dashboards need
   them. If the key→metric mapping lives in imperative Go (a `switch`, a builder per key),
   every new metric is a code change, a review, and a release — friction that discourages
   coverage and scatters naming decisions across functions.
2. **Cluster vs node scope.** Some keys are cluster-wide; others are reported per node and
   arrive tagged with a `devid` that must be resolved to a stable node LNN before the sample
   is labelled. The mapping has to carry that scope alongside the name.

A related constraint: most OneFS response *shapes* are validated against the OneFS 9.14.0
OpenAPI schema by a fixture guard (`schema_guard_test.go` / `onefs_schemas.json`), but the
statistics **key names** themselves are runtime values served by
`/platform/1/statistics/keys` and are *not* in the spec — they can only be confirmed against a
live cluster (see the `provisional-onefs-keys` memory note).

## Decision

**Express metric coverage as a curated, embedded JSON table — `statisticsKeys.json` — that
maps each OneFS stat key to a metric name and a scope. Adding a metric is a data edit, not a
code change.**

- **The table** (`statisticsKeys.json`, `//go:embed`-ed in `statkeys.go`) is a list of
  `{key, metric, scope}` rows, where `scope` is `"cluster"` or `"node"`. It is parsed once at
  init into `statKeySpecs` and a `statKeyByKey` index; an invalid table is a hard
  `log.Fatalf` at startup.
- **`QueryKeys()`** derives the distinct set of keys to request from OneFS directly from the
  table — the query and the mapping cannot drift, because they share one source.
- **`statSamples()`** (`derivations.go`) looks up each returned key in the table, skips keys
  not in the table, and branches on `scope`: `node`-scope samples resolve `devid`→LNN via the
  per-cluster `lnnByDevID` map (built from cluster inventory) and are dropped if the devid is
  unknown; `cluster`-scope samples get the base cluster labels.
- **Extending coverage = adding one row.** No Go change is needed for a new curated key; the
  unchecked Prometheus collector (see [ADR-0002](0002-snapshot-model-and-dual-export.md))
  emits whatever names the table produces.

### Alternatives considered

- **Imperative mapping in Go (a `switch` or per-key builder).** Rejected: every new metric
  becomes a code change; naming and scope decisions scatter across functions; the request
  list and the mapping can drift apart.
- **Auto-export every key OneFS returns.** Rejected: OneFS exposes hundreds of keys with
  unstable, non-self-describing names and no units; auto-export would produce an unstable,
  unprefixed, dashboard-hostile metric surface. Curation is the point — names carry units and
  match the csm-metrics-powerscale convention (see [ADR-0004](0004-powerscale-metric-prefix.md)).
- **A code-generated mapping from the OpenAPI spec.** Rejected: the stat **key names** are
  not in the spec (they are runtime values from `/statistics/keys`), so there is nothing to
  generate from; the table is hand-curated and validated against live clusters instead.
- **External/config-file table the operator edits.** Rejected: coverage is a property of the
  build, not a per-deployment knob; embedding it keeps the binary self-contained and the
  metric surface consistent across deployments.

## Consequences

**Positive**

- New metrics ship as a one-line data edit, reviewable at a glance.
- The query list and the name mapping share a single source — no drift.
- Scope (and the devid→LNN resolution) is declared next to each name, not buried in code.
- The binary is self-contained: the table is embedded, parsed and index-built at init.

**Negative / costs**

- The table is **not** covered by the schema-drift guard, because stat key *names* are not in
  the OneFS OpenAPI spec. New keys (notably the `cache.*` family) need live-cluster
  validation before they can be trusted; this is tracked in the `provisional-onefs-keys`
  memory note rather than enforced by a test.
- A typo in a `key` silently yields no samples (the key simply never matches a returned
  point) rather than a loud failure — caught only by `--once --debug` against a real cluster.
- Renaming a `metric` is a breaking change for downstream dashboards even though it looks like
  a trivial data edit; treat the table's `metric` column as a stable contract.

## References

- `internal/powerscale/statisticsKeys.json` — the curated table
- `internal/powerscale/statkeys.go` — embed, parse, index, `QueryKeys()`
- `internal/powerscale/derivations.go` — `statSamples()` scope branch and devid→LNN resolution
- `internal/powerscale/schema_guard_test.go` — why key names are out of scope for the guard
- [ADR-0002](0002-snapshot-model-and-dual-export.md) — the unchecked collector that emits the table's names
- [ADR-0004](0004-powerscale-metric-prefix.md) — the `powerscale_` naming convention the table follows
