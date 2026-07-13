# Design: OneFS storage-pool capacity metrics (#33)

Date: 2026-07-13
Status: approved for planning
Issue: #33 (second of three follow-up collectors from the v0.13.0 review; #34 license shipped as PR #35, #32 workload is a separate cycle)
Branch: `feat/storagepool-metrics`, stacked on `feat/license-metrics` (unmerged PR #35)
Scope: `internal/models/onefs.go`, `internal/powerscale/client.go`, `internal/powerscale/derivations.go`, `internal/powerscale/metrics.go`, `tools/extract-schemas/main.go`, `internal/powerscale/testdata/`, collector/e2e tests, `docs/metrics.md`, privilege docs.

## Goal

Expose per-pool and per-tier capacity so operators can alert on **SmartPools tiering headroom** and imbalance — the exporter today reports only cluster-wide and per-node `/ifs` capacity, nothing per storage/node pool or tier. This closes the coverage gap the Crest Data PowerScale Grafana datasource highlights (per-tier capacity / disk allocation).

## Why this is low-risk

Like the license collector, storage-pool capacity is **structural, schema-documented data** — the schema-drift guard fully validates every consumed field against the OneFS 9.14 OpenAPI spec. Better still, the one field that would otherwise be a runtime unknown, `type`, is **enumerated in the spec** as exactly `['tier', 'nodepool']`, so there is nothing to live-validate (contrast the license `status` string). No cluster-side prerequisite (contrast #32 workload). OneFS pre-computes all capacity numbers; the exporter does no arithmetic.

## Source

`GET /platform/1/storagepool/storagepools` — the **aggregate** list containing both node pools and tiers. Fetched **best-effort**, exactly like `syncPolicies`/`licenses` (a missing `ISI_PRIV_SMARTPOOLS` privilege or an older release simply yields no storage-pool metrics; the exporter and all other collectors keep running).

**Version choice — v1.** `storagepools` exists at v1/v3/v16; all three carry an identical item schema (`name`, `type`, `usage`) with identical `usage` fields. v1 is chosen for the widest OneFS backward compatibility (same rationale as license v5). The schema-drift guard validates the v1 path against the 9.14 spec.

Response shape (`{ "storagepools": [ … ], "total": N }`), per the 9.14 schema. Fields consumed per row:

| field | type | use |
| --- | --- | --- |
| `name` | string | pool/tier name → `pool` label |
| `type` | string (`nodepool`\|`tier`) | → `type` label |
| `usage.total_bytes` | **string** | `powerscale_storagepool_total_capacity_bytes` |
| `usage.used_bytes` | **string** | `powerscale_storagepool_used_capacity_bytes` |
| `usage.avail_bytes` | **string** | `powerscale_storagepool_available_capacity_bytes` |
| `usage.total_ssd_bytes` | **string** | `powerscale_storagepool_ssd_total_capacity_bytes` |
| `usage.used_ssd_bytes` | **string** | `powerscale_storagepool_ssd_used_capacity_bytes` |
| `usage.avail_ssd_bytes` | **string** | `powerscale_storagepool_ssd_available_capacity_bytes` |
| `usage.total_hdd_bytes` | **string** | `powerscale_storagepool_hdd_total_capacity_bytes` |
| `usage.used_hdd_bytes` | **string** | `powerscale_storagepool_hdd_used_capacity_bytes` |
| `usage.avail_hdd_bytes` | **string** | `powerscale_storagepool_hdd_available_capacity_bytes` |

**The `usage.*_bytes` fields are JSON strings** (e.g. `"1099511627776"`), not numbers. Parsing reuses the existing `flexFloat` type in `internal/models/onefs.go`, which already decodes a quoted OR bare JSON number and falls back to `0` on an unparseable value (logged at debug) — the same resilience the sensor readings use.

Not consumed: `usable_*`, `free_*`, `virtual_hot_spare_bytes`, `pct_used*`, `balanced`, `children`, `id`, `lnns`, `health_flags`, `l3*`, `protection_policy`, `manual`, `transfer_limit_*`, `node_type_ids`, and the top-level `total`/`resume`.

## Metrics

All best-effort; an empty/failed fetch emits nothing. Canonical leading labels `cluster`, `cluster_id`, then `pool`, `type`.

| Metric | Value |
| --- | --- |
| `powerscale_storagepool_total_capacity_bytes` | `usage.total_bytes` |
| `powerscale_storagepool_used_capacity_bytes` | `usage.used_bytes` |
| `powerscale_storagepool_available_capacity_bytes` | `usage.avail_bytes` |
| `powerscale_storagepool_ssd_total_capacity_bytes` | `usage.total_ssd_bytes` |
| `powerscale_storagepool_ssd_used_capacity_bytes` | `usage.used_ssd_bytes` |
| `powerscale_storagepool_ssd_available_capacity_bytes` | `usage.avail_ssd_bytes` |
| `powerscale_storagepool_hdd_total_capacity_bytes` | `usage.total_hdd_bytes` |
| `powerscale_storagepool_hdd_used_capacity_bytes` | `usage.used_hdd_bytes` |
| `powerscale_storagepool_hdd_available_capacity_bytes` | `usage.avail_hdd_bytes` |

**All 9 gauges are emitted for every row** (the schema always provides all `usage` fields; an all-HDD pool simply reports `ssd=0`). This matches the always-emit-even-0 pattern of `snapshotSamples`/`dedupeSamples`, so a series is distinguishable from missing data.

**Media split is encoded in the metric name, not a label.** This follows the project's unit-explicit distinct-name convention (cache `l1`/`l2`/`l3` are distinct names, not a `level` label) and avoids the footgun a `medium="all|ssd|hdd"` label would create — `sum()` over the label would double-count the aggregate against its parts.

**Double-counting note (the one correctness detail):** the aggregate list contains BOTH tiers and their child node pools, and a tier's capacity is the sum of its child node pools. Summing a metric across all rows therefore double-counts. The `type` label makes the hierarchy explicit; docs instruct users to filter `type="nodepool"` for a non-overlapping cluster-wide total. The exporter emits the rows as-is (no de-duplication) — this is expected for hierarchical capacity data.

Percentage is intentionally not emitted — left to PromQL (`used / total`), matching the existing `/ifs` capacity metrics.

## Data flow (established best-effort typed-collector pattern)

1. **`internal/models/onefs.go`**
   - `type StoragePool struct { Name, Type string; TotalBytes, UsedBytes, AvailBytes, SSDTotalBytes, SSDUsedBytes, SSDAvailBytes, HDDTotalBytes, HDDUsedBytes, HDDAvailBytes float64 }`
   - `func ParseStoragePools(b []byte) ([]StoragePool, error)` — unmarshal `{ "storagepools": [...] }` with a raw item whose `usage` sub-struct uses `flexFloat` fields for the string bytes; convert to `float64` in the output struct.
   - Add `StoragePools []StoragePool` to the `Inventory` struct.
2. **`internal/powerscale/client.go`**
   - New helper `func (c *ClusterClient) storagePools(ctx context.Context) []models.StoragePool`, modeled exactly on `licenses`/`syncPolicies`: `getRaw(ctx, "platform/1/storagepool/storagepools", &b)`; on error `log.Debugf(...); return nil`; else `ParseStoragePools`, on parse error `log.Debugf(...); return nil`.
   - Add `StoragePools: c.storagePools(ctx),` to the `Inventory{}` literal in `GetInventory`.
   - Extend the debug summary log with `storage_pools=%d` / `len(inv.StoragePools)`.
3. **`internal/powerscale/metrics.go`**
   - `func storagePoolLabels(clusterName, clusterID, pool, poolType string) []Label` — appends `pool` then `type` to `baseLabels`, following the existing `*Labels` helpers.
4. **`internal/powerscale/derivations.go`**
   - `func storagePoolSamples(clusterName, clusterID string, pools []models.StoragePool) []Sample` — for each pool, append all 9 gauges with `storagePoolLabels`.
   - Wire into `BuildSamples` via `inv.StoragePools`.

## Testing

- **`internal/powerscale/testdata/storagepools.json`** — rows exercising the hierarchy and media split:
  - a **tier** (`type:"tier"`, hybrid: non-zero ssd + hdd);
  - a **node pool** (`type:"nodepool"`, hybrid) that is a child of the tier;
  - an **all-HDD node pool** (`ssd` bytes = `"0"`) to exercise the `ssd=0` emission.
- **Schema guard:** add `"/platform/1/storagepool/storagepools": "storagepools.json"` to the `targets` map in `tools/extract-schemas/main.go`, then `make schemas` so `schema_guard_test.go` asserts every fixture field (`name`, `type`, `usage.total_bytes`, `usage.used_bytes`, `usage.avail_bytes`, and the ssd/hdd variants) is documented in the 9.14 spec.
- **Mock server:** add a `strings.HasSuffix(p, "/storagepool/storagepools")` case to `mockserver_test.go` serving `storagepools.json` via `writeBytes`.
- **Assertions:**
  - `models` unit test `TestParseStoragePools`: asserts string bytes parse to float64, the `type` values, and that the all-HDD pool has `SSDTotalBytes == 0`.
  - `derivations_test.go` `TestBuildSamplesStoragePools`: assert a `nodepool` row emits all 9 gauges with the right `pool`/`type` labels and values, and that the all-HDD pool emits `ssd_total=0`.
  - `e2e_test.go`: add the 9 `powerscale_storagepool_*` metrics to the presence map.
  - Follow the existing dual-path style (Prometheus registry gather + OTLP `ManualReader`).

## Docs

- New `## Storage pools` section in `docs/metrics.md`: the 9 metrics, the `pool`/`type` labels, the **double-counting note** (filter `type="nodepool"` for a cluster total), and a tiering-headroom alert example:

  ```promql
  # a node pool over 85% full
  100 * powerscale_storagepool_used_capacity_bytes
    / powerscale_storagepool_total_capacity_bytes > 85
  ```

- Add `ISI_PRIV_SMARTPOOLS` to the documented read-only privilege set (configuration.md / installation.md). Note it is best-effort: without the privilege, storage-pool metrics are simply absent.

## Non-goals

- `usable_bytes`, `free_bytes`, `virtual_hot_spare_bytes`, `pct_used*`, `balanced`, `children`, health flags, L3-cache flags, per-pool protection policy, node-type ids. YAGNI.
- Percentage metrics — derived in PromQL from used/total.
- Separate `/storagepool/nodepools` + `/storagepool/tiers` fetches — the aggregate `storagepools` endpoint already returns both with a `type` discriminator.
- #32 workload — a separate cycle.

## Risks

- **`ISI_PRIV_SMARTPOOLS` name** unconfirmed against Dell RBAC docs; if the privilege differs, the docs line is corrected and the best-effort design means collection is unaffected either way.
- **Endpoint version:** if a target OneFS predates v1 `/storagepool/storagepools` (extremely unlikely — v1 is the oldest), the fetch fails best-effort (no metrics), matching every other optional collector.
- **String-typed bytes:** handled by the existing `flexFloat` (unparseable → 0, logged at debug); a schema surprise cannot fail the collection.
