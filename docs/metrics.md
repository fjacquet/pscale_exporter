# Metrics Reference

All series use the **`powerscale_`** prefix — matching
[`dell/csm-metrics-powerscale`](https://github.com/dell/csm-metrics-powerscale) — and are
**gauges**. Metric names are unit-explicit: `_bytes`, `_bytes_per_second`,
`_operations_per_second`, `_microseconds`, `_percent`, `_seconds`.

!!! warning "Per-second gauges — do not `rate()`"
    `_operations_per_second` and `_bytes_per_second` metrics are already per-second
    gauges sampled from OneFS. Aggregate them with `sum`/`avg` in PromQL — **never**
    `rate()` or `irate()`. The `_total`-suffixed inventory counts are also gauges (current
    counts), not cumulative counters.

## Labels

Every metric carries the canonical leading labels `cluster` and `cluster_id`, with
scope-specific labels appended:

| Scope | Labels |
|---|---|
| Cluster | `cluster`, `cluster_id` |
| Node | `cluster`, `cluster_id`, `node` (logical node number / LNN) |
| Quota | `cluster`, `cluster_id`, `quota_id`, `quota_path`, `quota_type` |
| Protocol | `cluster`, `cluster_id`, `node`, `protocol` (e.g. `nfs3`, `smb2`), `op` |
| Health / meta | `cluster` only |

## Cluster metrics

| Metric | Unit | Description |
|---|---|---|
| `powerscale_cluster_cpu_sys_percent` | percent | System CPU usage. |
| `powerscale_cluster_cpu_user_percent` | percent | User CPU usage. |
| `powerscale_cluster_cpu_idle_percent` | percent | Idle CPU. |
| `powerscale_cluster_total_capacity_bytes` | bytes | Total `/ifs` filesystem capacity. |
| `powerscale_cluster_used_capacity_bytes` | bytes | Used capacity. |
| `powerscale_cluster_available_capacity_bytes` | bytes | Available (user-writable) capacity. |
| `powerscale_cluster_free_capacity_bytes` | bytes | Raw free capacity. |
| `powerscale_cluster_disk_operations_per_second` | ops/s | Cluster-wide disk transfer rate. |
| `powerscale_cluster_net_in_bytes_per_second` | bytes/s | External inbound network throughput. |
| `powerscale_cluster_net_out_bytes_per_second` | bytes/s | External outbound network throughput. |

## Node metrics

Per-node series; the `node` label is the logical node number (LNN).

| Metric | Unit | Description |
|---|---|---|
| `powerscale_node_cpu_idle_percent` | percent | Per-node idle CPU. |
| `powerscale_node_cpu_sys_percent` | percent | Per-node system CPU. |
| `powerscale_node_cpu_user_percent` | percent | Per-node user CPU. |
| `powerscale_node_memory_used_bytes` | bytes | Per-node memory used. |
| `powerscale_node_disk_operations_per_second` | ops/s | Per-node disk transfer rate. |
| `powerscale_node_used_capacity_bytes` | bytes | Per-node used `/ifs` capacity. |

### Cache

Per-node read-cache metrics for the L1/L2/L3 data-read path. Keys are node-scoped under the
OneFS `node.ifs.cache.*` namespace, confirmed against a live cluster via
`isi statistics list keys` (equivalently `GET /platform/1/statistics/keys`). Note that
`statistics/current` is **all-or-nothing**: a single invalid key fails the entire batch and
drops *all* current-statistics metrics — so any new key here must be validated against a live
cluster first. Compute hit ratio in PromQL as `hit / (hit + miss)`.

> Unit semantics (per-second **rate** vs cumulative **counter**) are pending live `--trace`
> validation; the `_bytes_per_second` suffix is provisional until confirmed (see the design
> spec, §3).

| Metric | Unit |
|---|---|
| `powerscale_node_cache_l1_read_hit_bytes_per_second` / `..._miss_...` | bytes/s (provisional) |
| `powerscale_node_cache_l2_read_hit_bytes_per_second` / `..._miss_...` | bytes/s (provisional) |
| `powerscale_node_cache_l3_read_hit_bytes_per_second` / `..._miss_...` | bytes/s (provisional) |

### Node health

| Metric | Unit | Description |
|---|---|---|
| `powerscale_node_readonly` | bool | `1` if the node is mounted read-only. |
| `powerscale_node_smartfail` | bool | `1` if the node is smartfailing / smartfailed. |
| `powerscale_node_drives_total` | count | Drive count per node, labelled by `state` (e.g. `HEALTHY`, `SMARTFAIL`, `DEAD`). |

### Hardware (provisional)

Per-node power-supply health and temperature/fan sensors, from the node `status` /
`sensors` payload. Best-effort; schema is provisional — emits nothing if your OneFS
release shapes these differently. Temperature/fan series carry a `sensor` label.

| Metric | Unit | Description |
|---|---|---|
| `powerscale_node_power_supplies_total` | count | Power supplies present on the node. |
| `powerscale_node_power_supply_failures` | count | Failed power supplies on the node. |
| `powerscale_node_temperature_celsius` | °C | Temperature sensor reading. |
| `powerscale_node_fan_speed_rpm` | rpm | Fan speed reading. |

## Quota metrics

| Metric | Unit | Description |
|---|---|---|
| `powerscale_quota_usage_bytes` | bytes | Logical usage for the quota. |
| `powerscale_quota_physical_usage_bytes` | bytes | Physical usage (post data-reduction). Emitted when > 0. |
| `powerscale_quota_hard_threshold_bytes` | bytes | Hard limit. Emitted only when set (> 0). |
| `powerscale_quota_soft_threshold_bytes` | bytes | Soft limit. Emitted only when set (> 0). |
| `powerscale_quota_advisory_threshold_bytes` | bytes | Advisory limit. Emitted only when set (> 0). |

## Protocol metrics

Per-node, per-protocol, per-operation, from the OneFS protocol summary.

| Metric | Unit | Description |
|---|---|---|
| `powerscale_protocol_operations_per_second` | ops/s | Operation rate for `protocol`/`op`. |
| `powerscale_protocol_latency_microseconds` | µs | Average latency for `protocol`/`op`. |

## Inventory counts

| Metric | Unit | Description |
|---|---|---|
| `powerscale_nfs_exports_total` | count | Number of NFS exports. |
| `powerscale_smb_shares_total` | count | Number of SMB shares. |
| `powerscale_snapshots_total` | count | Number of snapshots. |

## Data protection

All best-effort: a cluster without SyncIQ (or where the account lacks privilege) simply
emits no series.

| Metric | Unit | Description | Labels |
|---|---|---|---|
| `powerscale_snapshot_used_bytes` | bytes | Aggregate space held by snapshots. | `cluster`, `cluster_id` |
| `powerscale_synciq_policy_enabled` | bool | `1` if the SyncIQ replication policy is enabled. | + `policy` |
| `powerscale_synciq_last_run_failed` | bool | `1` if the policy's last run failed / needs attention. | + `policy` |
| `powerscale_active_events` | count | Unresolved OneFS event-group occurrences. | + `severity` |

## Storage efficiency

Cluster-wide deduplication, from `dedupe/dedupe-summary`. Best-effort; bytes are derived as
block counts × block size (validated against the OneFS 9.14.0 schema).

| Metric | Unit | Description |
|---|---|---|
| `powerscale_dedupe_logical_saved_bytes` | bytes | Logical space saved by deduplication. |
| `powerscale_dedupe_deduplicated_bytes` | bytes | Logical data that has been deduplicated. |

## Per-drive

From `statistics/summary/drive`. Best-effort. Labels: `cluster`,
`cluster_id`, `node`, `bay`, `type` (e.g. `SSD`, `HDD`).

| Metric | Unit | Description |
|---|---|---|
| `powerscale_drive_operations_per_second` | ops/s | Per-drive operation rate. |
| `powerscale_drive_busy_percent` | percent | Per-drive busy time. |

## Per-client

From `statistics/summary/client`, aggregated by `node` / `protocol` / `class` to bound
cardinality (individual remote clients are intentionally not exported). Best-effort.

| Metric | Unit | Description |
|---|---|---|
| `powerscale_client_operations_per_second` | ops/s | Operation rate per node/protocol/class. |
| `powerscale_client_in_bytes_per_second` | bytes/s | Inbound throughput. |
| `powerscale_client_out_bytes_per_second` | bytes/s | Outbound throughput. |

## Health & metadata

These carry only the `cluster` label.

| Metric | Unit | Description |
|---|---|---|
| `powerscale_up` | bool | `1` if the cluster was scraped successfully, `0` otherwise. |
| `powerscale_last_scrape_timestamp_seconds` | unix seconds | Time of the last successful collection. |
| `powerscale_cluster_api_version` | version | Detected OneFS platform API version. |

## Exporter metadata

Exporter-level, not tied to any cluster (no `cluster` label).

| Metric | Unit | Description |
|---|---|---|
| `pscale_exporter_build_info` | constant `1` | Exporter build information; the running version and Go version are carried in the `version` and `goversion` labels. |

## Example queries

```promql
# Cluster capacity used %, per cluster
100 * powerscale_cluster_used_capacity_bytes
  / powerscale_cluster_total_capacity_bytes

# Total protocol ops per cluster (per-second gauge — sum, never rate)
sum by (cluster) (powerscale_protocol_operations_per_second)

# Top 5 quotas by usage
topk(5, powerscale_quota_usage_bytes)

# Clusters currently down
powerscale_up == 0
```

These same series back the bundled [alert rules](deployment/docker.md) and the
[Grafana dashboard](dashboards.md).

## Extending coverage

To add a curated OneFS statistic, add a row to
`internal/powerscale/statisticsKeys.json` with `key`, `metric`, and `scope`
(`cluster` | `node`) — **no code change** is needed. Node-scope keys map a `devid` to a
node LNN automatically. Keep names unit-explicit and prefixed with `powerscale_`.
