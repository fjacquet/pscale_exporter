# PowerScale Exporter — Design Specification

**Date:** 2026-06-02
**Status:** Approved (design); pending implementation plan
**Foundation:** [`pflex_exporter`](https://github.com/fjacquet/pflex_exporter)
**References:** [dell/csm-metrics-powerscale](https://github.com/dell/csm-metrics-powerscale), [PowerScale OneFS Platform API (9.13)](https://developer.dell.com/apis/4088/versions/9.13.0/docs/1introduction.md), [dell/gopowerscale](https://github.com/dell/gopowerscale)

## Overview

A Go exporter for **Dell PowerScale (OneFS)** clusters that authenticates to the OneFS
Platform API, collects a broad set of capacity, inventory, and performance metrics, and
exposes them via **both** a Prometheus `/metrics` endpoint **and** an OTLP metric push. It
reuses the architecture of `pflex_exporter` (itself modeled on `nbu_exporter`): a single
background collection loop publishes an immutable snapshot that both export paths read.

One process monitors many PowerScale clusters; every metric carries a `cluster` label.

## Goals

- Broad OneFS coverage: cluster, nodes, protocols (NFS/SMB/S3), quotas, snapshots,
  storagepools, drives, and network/system performance.
- Dual export (Prometheus pull + OTLP push) at full parity, fed from one shared snapshot.
- Multi-cluster, hot config reload, graceful per-cluster degradation, optional OTel tracing.
- Maximum reuse of the proven `pflex_exporter` scaffold.

## Non-Goals

- Write operations against OneFS (read-only `monitor`-equivalent access only).
- Kubernetes PV→volume topology mapping (CSM's job; out of scope).
- Auto-discovering the entire OneFS statistics-key catalog (curated set instead).

## Key Decisions (locked during brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| Metric scope | **Broad OneFS coverage** | Match pflex breadth, not just CSM's capacity+perf subset. |
| API client | **Hybrid** — `gopowerscale` for typed resources, raw stats wrapper for the statistics API | Typed resources are well-served by the Dell SDK; the statistics API is curated by us. |
| Auth/session | **One shared authenticated session** | Mitigates the hybrid's "two stacks" risk: the raw stats wrapper reuses `gopowerscale`'s authenticated client + cookie jar. |
| Export paths | **Both (Prometheus + OTLP)** | Full parity with pflex. |
| Stats strategy | **Curated key list + summary endpoints** | Direct analog of pflex's `querySelectedStatistics.json`; predictable, extensible by editing JSON. |
| Metric prefix | **`powerscale_`** | Matches CSM (`dell/csm-metrics-powerscale`) so its Grafana dashboards work directly. |

## Architecture

### Snapshot model (inherited, unchanged)

A single background **collection loop** polls every configured cluster on
`collection.interval` and publishes an immutable **snapshot** to a `SnapshotStore`
(RWMutex pointer-swap). Both export paths read the latest snapshot rather than fetching on
scrape:

- **`PromCollector`** — an *unchecked* Prometheus collector (`Describe` sends nothing) so
  it can emit a dynamic metric-name set at `/metrics`.
- **`OTLPExporter`** — observable gauges driven by a periodic reader, pushed via OTLP.

This decouples OneFS API load from the number of scrapers and the OTLP push cadence.
`main.go` wires the HTTP server, the loop, hot config reload (SIGHUP + file watch), and a
snapshot-based `/health`.

### Reused scaffold (package renames only)

`main.go`, `internal/config` (watcher), `internal/models` (config / safe_config),
`internal/telemetry`, `internal/logging`, `internal/utils`, and the
snapshot / collector / prometheus / otlp / tracing files port over from
`internal/powerflex` with package renames and the new `Client` interface.

### New package: `internal/powerscale`

Replaces `internal/powerflex`. `Client` interface:

```go
type Client interface {
    Name() string
    // Typed inventory: nodes, quotas, NFS/SMB/S3 exports, snapshots, storagepools,
    // plus the parent/child relations graph.
    GetInventory(ctx context.Context) (*Inventory, *Relations, error)
    // Raw statistics: /statistics/current (curated keys) + /statistics/summary/*.
    GetStatistics(ctx context.Context) (*Statistics, error)
    // /platform/latest, detected once per cluster, cached.
    APIVersion(ctx context.Context) (int, error)
    Close() error
}
```

## Components

### Auth & HTTP (hybrid, one shared session)

- **`gopowerscale`** (`github.com/dell/gopowerscale`) owns the authenticated session:
  session-cookie auth with auto-renew before the 15-min idle / 4-hr absolute expiry. It
  serves all **typed** resources (quotas, NFS/SMB/S3, snapshots, nodes, storagepools).
- The **raw statistics API** reuses `gopowerscale`'s authenticated HTTP client (its
  `api.Client` issues arbitrary `GET`s). A thin request-builder constructs
  `/statistics/current?key=…&devid=all` and `/statistics/summary/*` against the same
  session → **one session, one cookie jar, two request builders.**
- Falls back to **HTTP Basic** if session auth is disabled on the cluster. Read-only; no
  CSRF needed (no writes).
- Retries exclude 4xx (never retry auth failures) — matching pflex's deliberate policy.

> Implementation note: before coding, verify `gopowerscale`'s exact surface — the raw-`GET`
> capability of its `api.Client`, and the quota/statistics struct shapes — via
> Context7/source rather than assumption.

### Statistics collection

- `internal/powerscale/statisticsKeys.json` — curated key list (cluster/node CPU, memory,
  disk op-rate & throughput, network, protocol op-rate & latency); the direct analog of
  pflex's `querySelectedStatistics.json`. Extending coverage = editing JSON.
- `/statistics/summary/{protocol,system,drive}` for pre-aggregated protocol performance.
- **API-version detection** replaces pflex's generation detection: `/platform/latest`
  resolved once per cluster, cached, used to build versioned paths (OneFS 8.1+ → 9.x).

### Metrics model

Unified `Sample{Name, []Label, Value}` → `powerscale_<obj>_<metric>{cluster, node, ...}`.

Object prefixes: `powerscale_cluster_`, `powerscale_node_`, `powerscale_quota_`,
`powerscale_nfs_`, `powerscale_smb_`, `powerscale_s3_`, `powerscale_snapshot_`,
`powerscale_storagepool_`, `powerscale_drive_`, `powerscale_protocol_`.

Where CSM (`dell/csm-metrics-powerscale`) defines an equivalent metric (e.g.
`powerscale_cluster_used_capacity_percentage`, `powerscale_volume_quota_subscribed_gigabytes`),
reuse its exact name and units so CSM Grafana dashboards work without modification; broader
OneFS metrics beyond CSM's set follow the same naming conventions.

Per-type label builders + a relations graph (cluster→node, cluster→quota→path,
cluster→export). Every series carries a `cluster` label.

**Conventions (inherited from pflex):**

- Units explicit in names: `_bytes`, `_bytes_per_second`, `_operations_per_second`,
  `_microseconds`, `_percent`.
- iops/bandwidth are already per-second gauges → aggregate with `sum`/`avg`, never `rate()`.
- A metric name must carry one label-key set across all series. Any object type produced by
  more than one builder emits a **union label set in a fixed canonical order**; a test
  guards mixed-source label consistency.

## Data Flow

1. Loop tick → per cluster (concurrent): resolve/cache API version → `GetInventory`
   (typed, via gopowerscale) + `GetStatistics` (raw, via shared session).
2. Build relations graph and per-object `Sample`s with resolved identity/parent labels.
3. Assemble per-cluster slice (`Up`, samples, timestamp) into a new immutable snapshot.
4. Pointer-swap into `SnapshotStore`.
5. `PromCollector` and `OTLPExporter` read the current snapshot independently.

## Error Handling & Resilience

- **Per-cluster graceful degradation:** a cluster that fails auth or collection is marked
  `Up=false` in its snapshot slice; other clusters keep reporting.
- **`/health`:** 200 if any cluster is up, 503 if all are down, 200 "starting" before the
  first cycle populates the store.
- **Retries:** bounded; 4xx excluded (auth failures are not retried).
- **Per-cluster collection timeout** (`collection.timeout`).

## Testing

- Mock OneFS server (`httptest` TLS) serving `/session/1/session`, `/platform/latest`, the
  typed resource endpoints, `/statistics/current`, and `/statistics/summary/*` from
  `testdata/` fixtures.
- Collector tests assert via **both** the Prometheus registry gather and an OTLP
  `ManualReader`.
- Label-key consistency test for any object type produced by more than one builder.
- Semgrep gate on every file write (use the `writeBytes(io.Writer, …)` helper for test
  handlers to satisfy the write-to-ResponseWriter rule). Dockerfiles declare a non-root
  `USER`.

## Repo Bootstrap & CI/CD

Copy the pflex scaffold into `pscale_exporter`:

- `Makefile` (build/test/ci/sbom/release targets), `Dockerfile` (non-root `USER`),
  `.github/workflows/` (ci / release / docs), `mkdocs.yml`, `grafana/`, `config.yaml`.
- `go.mod` module path `github.com/fjacquet/pscale_exporter`; add `github.com/dell/gopowerscale`.
- Config schema nearly identical to pflex's `clusters[]`; gains `port` (default 8080),
  drops gateway-specific fields. Passwords via `${ENV_VAR}` interpolation or `passwordFile`.

### Example config sketch

```yaml
collection:
  interval: "30s"   # capacity stats update slowly; perf ~30s native sample length
  timeout: "20s"
clusters:
  - name: pscale-cluster1
    endpoint: pscale-clu1.example.com
    port: 8080
    username: pscale-monitor
    password: "${PSCALE1_PASSWORD}"
    insecureSkipVerify: true
```

## Open Items for Implementation Phase

- Confirm `gopowerscale` `api.Client` raw-`GET` capability and quota/statistics struct shapes.
- Finalize the curated `statisticsKeys.json` contents against a live OneFS keys catalog.
- Decide collection interval default (capacity changes slowly; CPU samples at ~30s native).
