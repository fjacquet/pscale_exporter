# Design: OneFS per-workload performance metrics (#32)

Date: 2026-07-13
Status: approved for planning
Issue: #32 (third of three follow-up collectors from the v0.13.0 review; #34 license = PR #35, #33 storagepool = PR #36)
Branch: `feat/workload-metrics`, stacked on `feat/storagepool-metrics` (unmerged PR #36, itself stacked on PR #35)
Scope: `internal/models/onefs.go`, `internal/powerscale/client.go`, `internal/powerscale/derivations.go`, `internal/powerscale/metrics.go`, `tools/extract-schemas/main.go`, `internal/powerscale/testdata/`, collector/e2e tests, `docs/metrics.md`.

## Goal

Expose per-workload performance (operations, throughput, CPU) so operators can attribute cluster load to a specific access zone, protocol, user, or job — the parity gap the Crest Data PowerScale Grafana datasource highlights ("Workload Summary"). This is the most complex of the three follow-ups: the payload carries many identity dimensions (cardinality must be bounded) and depends on a cluster-side prerequisite.

## Why this is the most involved of the three

Unlike license (enumerated `type`/`status`) and storagepool (fixed capacity fields), the workload payload has ~15 identity dimensions ranging from low-cardinality (`node`, `zone_name`, `protocol`) to genuinely unbounded (`path`, IP addresses, SIDs), and **which dimensions are populated depends on the operator's performance-dataset definition**. The design therefore commits to a *curated, fixed* label set (Prometheus requires a consistent label-name set per metric) and excludes the unbounded dimensions.

## Source

`GET /platform/4/statistics/summary/workload`, fetched **best-effort** (like the drive/client summaries). This is a **statistics summary**, so it belongs on the `Statistics` struct (`st.Workloads`), fetched in `GetStatistics` alongside `Drives`/`Clients` — NOT on `Inventory`.

**Version choice — v4.** The endpoint exists at v4/v9/v10 in **both** the 9.13.0 and 9.14.0 specs; all three carry every field this design consumes (`ops`, `bytes_in`, `bytes_out`, `cpu`, `node`, `zone_name`, `protocol`, `username`, `system_name`, `job_type`). v4 is pinned for the widest OneFS compatibility (v9/v10 only add fields we don't use). The schema-drift guard validates the v4 path against the 9.14 spec.

**Privilege: `ISI_PRIV_STATISTICS`** — already required and documented for the other statistics endpoints; **no new privilege**.

**Prerequisite (must be documented):** workload rows are produced by OneFS *performance datasets* (`isi performance datasets`). Without a configured dataset, the endpoint returns few or only aggregate rows. Best-effort means a cluster without datasets simply yields no/minimal workload series — the exporter and all other collectors keep running.

Response shape (`{ "workload": [ … ] }`), per the 9.14 schema. Fields consumed per row:

| field | type | use |
| --- | --- | --- |
| `node` | number | `node` label (row's node; `0` = cluster-scoped) |
| `zone_name` | string,null | `zone` label |
| `protocol` | string,null | `protocol` label |
| `username` | string,null | `username` label |
| `system_name` | string,null | `system_name` label (process name / job id) |
| `job_type` | string,null | `job_type` label |
| `ops` | number | `powerscale_workload_operations_per_second` |
| `bytes_in` | number | `powerscale_workload_in_bytes_per_second` |
| `bytes_out` | number | `powerscale_workload_out_bytes_per_second` |
| `cpu` | number | `powerscale_workload_cpu_microseconds` (µs of CPU per second) |

The perf fields are JSON **numbers** (not strings, unlike storagepool) — decode to plain `float64`, no `flexFloat`. The nullable string dimensions decode to `""` on JSON `null` (Go's default), giving the empty-label behaviour the fixed label set needs.

Not consumed (excluded by design): `path`, `local_address`/`local_name`, `remote_address`/`remote_name`, `user_sid`/`group_sid`/`user_id`/`group_id`/`groupname`, `zone_id`, `domain_id`, `export_id`, `share_name`, `workload_id`, `workload_type`, `error`, `time`, `reads`, `writes`, `l2`, `l3`, `latency_read`/`latency_write`/`latency_other`.

## Metrics

All best-effort; an empty/failed fetch emits nothing. Canonical leading labels `cluster`, `cluster_id`, then the fixed workload label set.

| Metric | Value | Unit |
| --- | --- | --- |
| `powerscale_workload_operations_per_second` | `ops` | operations/sec |
| `powerscale_workload_in_bytes_per_second` | `bytes_in` | bytes/sec |
| `powerscale_workload_out_bytes_per_second` | `bytes_out` | bytes/sec |
| `powerscale_workload_cpu_microseconds` | `cpu` | µs of CPU time per second, across all cores |

**All four are per-second gauges** — aggregate with `sum`/`avg`, never `rate()` (consistent with the exporter's other per-second gauges). `cpu` is microseconds-of-CPU-per-second (a busy-ness rate, not a cumulative counter); the metric name carries `_microseconds` and the docs state explicitly it is per-second.

Every metric is emitted for every returned workload row (no conditional skipping); a row with an unpopulated dimension simply carries `""` for that label.

## Labels

**Fixed 8-label set** (Prometheus requires a consistent label-name set per metric name): `cluster`, `cluster_id`, `node`, `zone`, `protocol`, `username`, `system_name`, `job_type`.

- `node` — the row's `node` number rendered as a string (`"0"` = cluster-scoped). Workload rows report `node` directly, so there is **no `devid`→LNN mapping** (contrast the curated `statistics/current` node keys).
- `zone` ← `zone_name`; `protocol`, `username`, `system_name`, `job_type` map 1:1. Any unpinned dimension is `""`.

**Cardinality.** The unbounded dimensions (`path`, IP addresses, SIDs) are deliberately excluded. Remaining cardinality is governed by how the operator defines their performance dataset (a dataset pinned on `{protocol, username}` yields one series per protocol×user). No hard row cap is imposed in v1 — the operator controls cardinality through dataset design; the docs call this out as the tuning lever.

## Data flow (established best-effort typed-collector pattern)

1. **`internal/models/onefs.go`**
   - `type Workload struct { Node int; Zone, Protocol, Username, SystemName, JobType string; Ops, BytesIn, BytesOut, CPUMicros float64 }`
   - `func ParseWorkloadSummary(b []byte) ([]Workload, error)` — unmarshal `{ "workload": [...] }`. The raw `node` decodes via `float64` then converts to `int` (robust against a `1.0`-style JSON number); string dims map straight through (`null` → `""`).
   - Add `Workloads []Workload` to the `Statistics` struct (alongside `Drives`, `Clients`).
2. **`internal/powerscale/client.go`**
   - New helper `func (c *ClusterClient) workloadSummary(ctx context.Context) []models.Workload`, modeled on `driveSummary`/`clientSummary`: `getRaw(ctx, "platform/4/statistics/summary/workload", &b)`; on error `log.Debugf(...); return nil`; else `ParseWorkloadSummary`, on parse error `log.Debugf(...); return nil`.
   - In `GetStatistics`, set `st.Workloads = c.workloadSummary(ctx)` (next to `st.Drives`/`st.Clients`).
   - Extend the `GetStatistics` debug summary log with `workload_rows=%d` / `len(st.Workloads)`.
3. **`internal/powerscale/metrics.go`**
   - `func workloadLabels(clusterName, clusterID, node, zone, protocol, username, systemName, jobType string) []Label` — appends the six workload dimensions to `baseLabels`, following the existing `*Labels` helpers.
4. **`internal/powerscale/derivations.go`**
   - `func workloadSamples(clusterName, clusterID string, st *models.Statistics) []Sample` — guards `st == nil` (like `driveSamples`); for each `st.Workloads` row, appends the 4 gauges with `workloadLabels` (node via `strconv.Itoa`).
   - Wire into `BuildSamples`.

## Testing

- **`internal/powerscale/testdata/stat_workload.json`** — two rows exercising both label states:
  - a **dataset-pinned** row: `node` 1, `zone_name` "System", `protocol` "nfs3", `username` "alice", `system_name`/`job_type` empty, with non-zero `ops`/`bytes_in`/`bytes_out`/`cpu`;
  - an **aggregate** row: `node` 0, all string dims absent/`null`, with its own perf values — exercises the empty-label path.
- **Schema guard:** add `"/platform/4/statistics/summary/workload": "stat_workload.json"` to the `targets` map in `tools/extract-schemas/main.go`, then `make schemas` so `schema_guard_test.go` asserts every fixture field (`node`, `zone_name`, `protocol`, `username`, `system_name`, `job_type`, `ops`, `bytes_in`, `bytes_out`, `cpu`) is documented in the 9.14 v4 schema.
- **Mock server:** add a `strings.HasSuffix(p, "/statistics/summary/workload")` case to `mockserver_test.go` serving `stat_workload.json` via `writeBytes`.
- **Assertions:**
  - `models` unit test `TestParseWorkloadSummary`: string dims parse, `null`→`""`, `node` 0 and 1, perf values.
  - `derivations_test.go` `TestBuildSamplesWorkloads`: assert the pinned row emits all 4 gauges with the right label values, and the aggregate row emits `zone=""`/`protocol=""` (empty-label path) while still carrying values.
  - `e2e_test.go`: add the 4 `powerscale_workload_*` metrics to the presence map.
  - Follow the existing dual-path style (Prometheus registry gather + OTLP `ManualReader`).

## Docs

- New `## Workloads` section in `docs/metrics.md`: the 4 metrics, the 8-label set, a **prominent prerequisite note** (workload metrics require configured OneFS performance datasets — `isi performance datasets`; absent otherwise), the cardinality guidance (dataset design is the tuning lever; `path`/IP dimensions intentionally omitted), that `cpu` is µs-of-CPU-per-second (a per-second gauge), and that **latency is a planned follow-up**. Example:

  ```promql
  # top 5 workloads by operations/sec, per cluster
  topk(5, sum by (cluster, zone, username, protocol) (powerscale_workload_operations_per_second))
  ```

- **No privilege-doc change** — `ISI_PRIV_STATISTICS` is already documented.

## Non-goals

- **Latency** (`latency_read`/`latency_write`/`latency_other`) — deferred to a small follow-up: the 9.14 schema states no unit for these fields (OneFS convention is microseconds, unconfirmed), so shipping them now would be an unverified metric. Add once a live workload body confirms the unit.
- `reads`, `writes` (disk ops/s) and `l2`, `l3` (cache hits/s) — YAGNI for the first cut.
- The high-cardinality labels (`path`, IPs, SIDs, `share_name`, `export_id`, `group*`, `domain_id`) and pinned-`workload_id` labels.
- A hard row/cardinality cap — the operator controls cardinality via dataset design; documented as the tuning lever instead.

## Risks

- **`cpu` unit semantics** — the schema states "micro-seconds per second across all cores." The value is a rate; the name `_microseconds` is documented as per-second to avoid a `rate()` misuse. If a live body shows a different scale, the docs are corrected (best-effort, so collection is unaffected either way).
- **Live verification pending** — like the other collectors, the structure is schema-validated but not live-verified until the operator configures a performance dataset and captures a trace. The best-effort design means any field surprise yields no/partial metrics rather than a failure.
- **Empty-string labels** — unpinned dimensions render as `""`. This is standard Prometheus and keeps the label-name set consistent across rows; documented so it isn't surprising.
- **Endpoint version** — if a target OneFS predates v4 `/statistics/summary/workload` (unlikely — v4 is the oldest present in 9.13/9.14), the fetch fails best-effort (no metrics), matching every other optional collector.
