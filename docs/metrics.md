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
| `powerscale_node_memory_used_bytes` | bytes | Per-node memory used. |
| `powerscale_node_disk_operations_per_second` | ops/s | Per-node disk transfer rate. |
| `powerscale_node_used_capacity_bytes` | bytes | Per-node used `/ifs` capacity. |

## Quota metrics

| Metric | Unit | Description |
|---|---|---|
| `powerscale_quota_usage_bytes` | bytes | Current usage for the quota. |
| `powerscale_quota_hard_threshold_bytes` | bytes | Hard limit. Emitted only when a hard threshold is set (> 0). |

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

## Health & metadata

These carry only the `cluster` label.

| Metric | Unit | Description |
|---|---|---|
| `powerscale_up` | bool | `1` if the cluster was scraped successfully, `0` otherwise. |
| `powerscale_last_scrape_timestamp_seconds` | unix seconds | Time of the last successful collection. |
| `powerscale_cluster_api_version` | version | Detected OneFS platform API version. |

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
