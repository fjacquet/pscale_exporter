# OneFS Storage-Pool Capacity Metrics (#33) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose per-pool and per-tier OneFS capacity (`powerscale_storagepool_{,ssd_,hdd_}{total,used,available}_capacity_bytes`) so operators can alert on SmartPools tiering headroom and imbalance.

**Architecture:** The established best-effort typed-collector pattern — a `models.StoragePool` type + `ParseStoragePools`, a best-effort `c.storagePools(ctx)` fetch wired into `GetInventory`, and a `storagePoolSamples` builder in `derivations.go`. No new architecture; mirrors the license collector (`licenses`/`licenseSamples`) shipped in the parent branch.

**Tech Stack:** Go, `go test`, `httptest` mock OneFS server, the CycloneDX schema-drift guard (`make schemas`), Prometheus client_golang.

## Global Constraints

- Keep the `powerscale_` metric prefix; metric names are unit/meaning-explicit (`_bytes`).
- Canonical leading labels on every sample: `cluster`, then `cluster_id` (via `baseLabels`), then `pool`, `type`.
- **Best-effort collector:** a fetch/parse error logs at debug and returns `nil` — it never fails the inventory (same as `licenses`, `client.go`).
- **The `usage.*_bytes` fields are JSON strings**, not numbers — parse them through the existing `flexFloat` type in `internal/models/onefs.go` (quoted or bare number, unparseable → 0, logged at debug).
- **Media split is in the metric name, not a label** (`_ssd_`/`_hdd_`), following the cache `l1`/`l2`/`l3` convention; all 9 gauges are emitted for every pool (an all-HDD pool reports `ssd=0`).
- **The list contains both node pools and tiers** (a tier = sum of its child node pools); rows are emitted as-is with a `type` label (`nodepool`|`tier`) — no de-duplication. Docs tell users to filter `type="nodepool"` for a cluster total.
- Fixtures are validated against the OneFS 9.14 OpenAPI spec by the schema-drift guard (`make schemas`); every fixture field must be documented in the spec.
- A Semgrep hook scans files on write and **blocks on findings**; inline `// nosemgrep` is not honored. Test HTTP handlers must write through the `writeBytes(io.Writer, …)` helper (already in `mockserver_test.go`), never directly to a `ResponseWriter`.
- Branch: `feat/storagepool-metrics` (already created; stacked on `feat/license-metrics`; the spec is committed there).

---

### Task 1: StoragePool model + `ParseStoragePools`

**Files:**

- Modify: `internal/models/onefs.go` (add `StoragePool` type, `ParseStoragePools`, `Inventory.StoragePools` field)
- Test: `internal/models/onefs_test.go`

**Interfaces:**

- Consumes: `encoding/json`, the existing unexported `flexFloat` type (both already in `onefs.go`).
- Produces: `type StoragePool struct { Name, Type string; TotalBytes, UsedBytes, AvailBytes, SSDTotalBytes, SSDUsedBytes, SSDAvailBytes, HDDTotalBytes, HDDUsedBytes, HDDAvailBytes float64 }`; `func ParseStoragePools(b []byte) ([]StoragePool, error)`; `Inventory.StoragePools []StoragePool`.

- [ ] **Step 1: Write the failing test**

Add to `internal/models/onefs_test.go` (place after `TestParseLicenses`):

```go
func TestParseStoragePools(t *testing.T) {
 data := []byte(`{"storagepools":[
  {"name":"tier1","type":"tier","usage":{"total_bytes":"3000","used_bytes":"1000","avail_bytes":"2000","total_ssd_bytes":"1000","used_ssd_bytes":"400","avail_ssd_bytes":"600","total_hdd_bytes":"2000","used_hdd_bytes":"600","avail_hdd_bytes":"1400"}},
  {"name":"h500_nodepool","type":"nodepool","usage":{"total_bytes":"2000","used_bytes":"600","avail_bytes":"1400","total_ssd_bytes":"0","used_ssd_bytes":"0","avail_ssd_bytes":"0","total_hdd_bytes":"2000","used_hdd_bytes":"600","avail_hdd_bytes":"1400"}}
 ]}`)
 ps, err := ParseStoragePools(data)
 if err != nil || len(ps) != 2 {
  t.Fatalf("parse: %d err=%v", len(ps), err)
 }
 if ps[0].Name != "tier1" || ps[0].Type != "tier" || ps[0].TotalBytes != 3000 || ps[0].SSDUsedBytes != 400 {
  t.Fatalf("pool[0] (string bytes must parse): %+v", ps[0])
 }
 if ps[1].Type != "nodepool" || ps[1].SSDTotalBytes != 0 || ps[1].HDDTotalBytes != 2000 {
  t.Fatalf("pool[1] (all-HDD): %+v", ps[1])
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestParseStoragePools -v`
Expected: FAIL — `undefined: ParseStoragePools` / `undefined: StoragePool`.

- [ ] **Step 3: Implement the model + parser**

In `internal/models/onefs.go`, add the field to the `Inventory` struct (after `Licenses []License`):

```go
 StoragePools []StoragePool
```

And add the type + parser (place near `ParseLicenses`):

```go
// StoragePool is one OneFS storage pool or tier (storagepool/storagepools). Both node pools
// and tiers appear in the list, distinguished by Type ("nodepool" | "tier"); a tier's
// capacity is the sum of its child node pools. The SSD/HDD fields break the aggregate down
// by media (an all-HDD pool reports zero SSD bytes).
type StoragePool struct {
 Name          string
 Type          string
 TotalBytes    float64
 UsedBytes     float64
 AvailBytes    float64
 SSDTotalBytes float64
 SSDUsedBytes  float64
 SSDAvailBytes float64
 HDDTotalBytes float64
 HDDUsedBytes  float64
 HDDAvailBytes float64
}

// ParseStoragePools parses storagepool/storagepools. The usage byte fields are JSON strings
// in the OneFS schema, so they decode through flexFloat (quoted or bare number, unparseable
// → 0).
func ParseStoragePools(b []byte) ([]StoragePool, error) {
 var raw struct {
  StoragePools []struct {
   Name  string `json:"name"`
   Type  string `json:"type"`
   Usage struct {
    TotalBytes    flexFloat `json:"total_bytes"`
    UsedBytes     flexFloat `json:"used_bytes"`
    AvailBytes    flexFloat `json:"avail_bytes"`
    TotalSSDBytes flexFloat `json:"total_ssd_bytes"`
    UsedSSDBytes  flexFloat `json:"used_ssd_bytes"`
    AvailSSDBytes flexFloat `json:"avail_ssd_bytes"`
    TotalHDDBytes flexFloat `json:"total_hdd_bytes"`
    UsedHDDBytes  flexFloat `json:"used_hdd_bytes"`
    AvailHDDBytes flexFloat `json:"avail_hdd_bytes"`
   } `json:"usage"`
  } `json:"storagepools"`
 }
 if err := json.Unmarshal(b, &raw); err != nil {
  return nil, err
 }
 out := make([]StoragePool, 0, len(raw.StoragePools))
 for _, p := range raw.StoragePools {
  out = append(out, StoragePool{
   Name:          p.Name,
   Type:          p.Type,
   TotalBytes:    float64(p.Usage.TotalBytes),
   UsedBytes:     float64(p.Usage.UsedBytes),
   AvailBytes:    float64(p.Usage.AvailBytes),
   SSDTotalBytes: float64(p.Usage.TotalSSDBytes),
   SSDUsedBytes:  float64(p.Usage.UsedSSDBytes),
   SSDAvailBytes: float64(p.Usage.AvailSSDBytes),
   HDDTotalBytes: float64(p.Usage.TotalHDDBytes),
   HDDUsedBytes:  float64(p.Usage.UsedHDDBytes),
   HDDAvailBytes: float64(p.Usage.AvailHDDBytes),
  })
 }
 return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -run TestParseStoragePools -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/onefs.go internal/models/onefs_test.go
git commit -m "feat(models): StoragePool type and ParseStoragePools"
```

---

### Task 2: `storagePoolSamples` builder + label helper

**Files:**

- Modify: `internal/powerscale/metrics.go` (add `storagePoolLabels`)
- Modify: `internal/powerscale/derivations.go` (add `storagePoolSamples`, wire into `BuildSamples`)
- Test: `internal/powerscale/derivations_test.go`

**Interfaces:**

- Consumes: `models.StoragePool` (Task 1); `baseLabels`, `Sample`, `Label` (existing).
- Produces: `storagePoolLabels(clusterName, clusterID, pool, poolType string) []Label`; `storagePoolSamples(clusterName, clusterID string, pools []models.StoragePool) []Sample`; emits the 9 metric names `powerscale_storagepool_{,ssd_,hdd_}{total,used,available}_capacity_bytes`.

- [ ] **Step 1: Write the failing test**

Add to `internal/powerscale/derivations_test.go` (place after `TestBuildSamplesLicenses`):

```go
func TestBuildSamplesStoragePools(t *testing.T) {
 inv := &models.Inventory{
  Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
  StoragePools: []models.StoragePool{
   {Name: "nodepool1", Type: "nodepool", TotalBytes: 3000, UsedBytes: 1000, AvailBytes: 2000,
    SSDTotalBytes: 1000, SSDUsedBytes: 400, SSDAvailBytes: 600,
    HDDTotalBytes: 2000, HDDUsedBytes: 600, HDDAvailBytes: 1400},
   {Name: "hdd_pool", Type: "nodepool", TotalBytes: 2000, UsedBytes: 600, AvailBytes: 1400,
    SSDTotalBytes: 0, SSDUsedBytes: 0, SSDAvailBytes: 0,
    HDDTotalBytes: 2000, HDDUsedBytes: 600, HDDAvailBytes: 1400},
  },
 }
 samples := BuildSamples("clu1", inv, nil)
 find := func(name, pool string) (Sample, bool) {
  for _, s := range samples {
   if s.Name != name {
    continue
   }
   for _, l := range s.Labels {
    if l.Name == "pool" && l.Value == pool {
     return s, true
    }
   }
  }
  return Sample{}, false
 }
 if s, ok := find("powerscale_storagepool_total_capacity_bytes", "nodepool1"); !ok || s.Value != 3000 {
  t.Fatalf("nodepool1 total wrong: %+v ok=%v", s, ok)
 }
 if s, ok := find("powerscale_storagepool_ssd_used_capacity_bytes", "nodepool1"); !ok || s.Value != 400 {
  t.Fatalf("nodepool1 ssd_used wrong: %+v ok=%v", s, ok)
 }
 // the all-HDD pool still emits an ssd_total series, valued 0 (always-emit)
 if s, ok := find("powerscale_storagepool_ssd_total_capacity_bytes", "hdd_pool"); !ok || s.Value != 0 {
  t.Fatalf("hdd_pool ssd_total should be present and 0: %+v ok=%v", s, ok)
 }
 // the type label is present
 s, ok := find("powerscale_storagepool_total_capacity_bytes", "nodepool1")
 hasType := false
 for _, l := range s.Labels {
  if l.Name == "type" && l.Value == "nodepool" {
   hasType = true
  }
 }
 if !ok || !hasType {
  t.Fatalf("nodepool1 missing type label: %+v", s.Labels)
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesStoragePools -v`
Expected: FAIL — `storagePoolSamples` not wired / metrics absent.

- [ ] **Step 3: Add the label helper**

In `internal/powerscale/metrics.go` (after `licenseInfoLabels`):

```go
// storagePoolLabels appends a storage-pool/tier name and its type (nodepool|tier).
func storagePoolLabels(clusterName, clusterID, pool, poolType string) []Label {
 return append(baseLabels(clusterName, clusterID),
  Label{Name: "pool", Value: pool},
  Label{Name: "type", Value: poolType},
 )
}
```

- [ ] **Step 4: Add `storagePoolSamples` and wire it in**

In `internal/powerscale/derivations.go`, add the builder (place after `licenseSamples`):

```go
// storagePoolSamples emits per-pool/per-tier capacity: the aggregate plus an SSD/HDD media
// split. The list contains both node pools and tiers (a tier's capacity is the sum of its
// child node pools), distinguished by the type label — summing across all rows double-counts,
// so consumers filter type="nodepool" for a non-overlapping cluster total. All 9 gauges are
// always emitted (an all-HDD pool simply reports ssd=0).
func storagePoolSamples(clusterName, clusterID string, pools []models.StoragePool) []Sample {
 var out []Sample
 for _, p := range pools {
  labels := storagePoolLabels(clusterName, clusterID, p.Name, p.Type)
  out = append(out,
   Sample{Name: "powerscale_storagepool_total_capacity_bytes", Labels: labels, Value: p.TotalBytes},
   Sample{Name: "powerscale_storagepool_used_capacity_bytes", Labels: labels, Value: p.UsedBytes},
   Sample{Name: "powerscale_storagepool_available_capacity_bytes", Labels: labels, Value: p.AvailBytes},
   Sample{Name: "powerscale_storagepool_ssd_total_capacity_bytes", Labels: labels, Value: p.SSDTotalBytes},
   Sample{Name: "powerscale_storagepool_ssd_used_capacity_bytes", Labels: labels, Value: p.SSDUsedBytes},
   Sample{Name: "powerscale_storagepool_ssd_available_capacity_bytes", Labels: labels, Value: p.SSDAvailBytes},
   Sample{Name: "powerscale_storagepool_hdd_total_capacity_bytes", Labels: labels, Value: p.HDDTotalBytes},
   Sample{Name: "powerscale_storagepool_hdd_used_capacity_bytes", Labels: labels, Value: p.HDDUsedBytes},
   Sample{Name: "powerscale_storagepool_hdd_available_capacity_bytes", Labels: labels, Value: p.HDDAvailBytes},
  )
 }
 return out
}
```

Wire it into `BuildSamples` (add immediately after the `licenseSamples(...)` append line):

```go
 samples = append(samples, storagePoolSamples(clusterName, clusterID, inv.StoragePools)...)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesStoragePools -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/powerscale/metrics.go internal/powerscale/derivations.go internal/powerscale/derivations_test.go
git commit -m "feat(powerscale): storagePoolSamples builder for storage-pool metrics"
```

---

### Task 3: Best-effort fetch, fixture, schema guard, and e2e coverage

**Files:**

- Modify: `internal/powerscale/client.go` (add `storagePools` helper + `Inventory` literal + debug log)
- Create: `internal/powerscale/testdata/storagepools.json`
- Modify: `tools/extract-schemas/main.go` (targets map) — then regenerate via `make schemas`
- Modify: `internal/powerscale/mockserver_test.go` (serve the fixture)
- Modify: `internal/powerscale/e2e_test.go` (presence map)

**Interfaces:**

- Consumes: `models.ParseStoragePools`, `Inventory.StoragePools` (Task 1); the emitted metric names (Task 2); `c.getRaw`, `snippet`, `log` (existing).
- Produces: `func (c *ClusterClient) storagePools(ctx context.Context) []models.StoragePool`; the `storagepools.json` fixture; end-to-end emission of the 9 storage-pool metrics.

- [ ] **Step 1: Add the 9 storage-pool metrics to the e2e presence map (failing first)**

In `internal/powerscale/e2e_test.go`, add to the `want := map[string]bool{ … }` block (after the `powerscale_license_info` line):

```go
  "powerscale_storagepool_total_capacity_bytes":         false,
  "powerscale_storagepool_used_capacity_bytes":          false,
  "powerscale_storagepool_available_capacity_bytes":     false,
  "powerscale_storagepool_ssd_total_capacity_bytes":     false,
  "powerscale_storagepool_ssd_used_capacity_bytes":      false,
  "powerscale_storagepool_ssd_available_capacity_bytes": false,
  "powerscale_storagepool_hdd_total_capacity_bytes":     false,
  "powerscale_storagepool_hdd_used_capacity_bytes":      false,
  "powerscale_storagepool_hdd_available_capacity_bytes": false,
```

Then run `gofmt -w internal/powerscale/e2e_test.go` to fix column alignment.

- [ ] **Step 2: Run the e2e test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: FAIL — `missing metric powerscale_storagepool_*` (nothing fetches/serves storage pools yet).

- [ ] **Step 3: Add the fixture**

Create `internal/powerscale/testdata/storagepools.json`. The three rows form a consistent hierarchy (tier1 total = h500 + f200 for every field), and h500 exercises the `ssd=0` case:

```json
{"storagepools": [
  {"name": "tier1", "type": "tier", "usage": {"total_bytes": "3000", "used_bytes": "1000", "avail_bytes": "2000", "total_ssd_bytes": "1000", "used_ssd_bytes": "400", "avail_ssd_bytes": "600", "total_hdd_bytes": "2000", "used_hdd_bytes": "600", "avail_hdd_bytes": "1400"}},
  {"name": "h500_nodepool", "type": "nodepool", "usage": {"total_bytes": "2000", "used_bytes": "600", "avail_bytes": "1400", "total_ssd_bytes": "0", "used_ssd_bytes": "0", "avail_ssd_bytes": "0", "total_hdd_bytes": "2000", "used_hdd_bytes": "600", "avail_hdd_bytes": "1400"}},
  {"name": "f200_nodepool", "type": "nodepool", "usage": {"total_bytes": "1000", "used_bytes": "400", "avail_bytes": "600", "total_ssd_bytes": "1000", "used_ssd_bytes": "400", "avail_ssd_bytes": "600", "total_hdd_bytes": "0", "used_hdd_bytes": "0", "avail_hdd_bytes": "0"}}
]}
```

- [ ] **Step 4: Serve the fixture from the mock server**

In `internal/powerscale/mockserver_test.go`, add a case to the path switch (before `default`), mirroring the existing cases:

```go
  case strings.HasSuffix(p, "/storagepool/storagepools"):
   writeBytes(w, fixture(t, "storagepools.json"))
```

- [ ] **Step 5: Add the best-effort fetch + wire into `GetInventory`**

In `internal/powerscale/client.go`, add the helper (place after `licenses`):

```go
// storagePools fetches per-pool/per-tier capacity best-effort (a missing ISI_PRIV_SMARTPOOLS
// privilege or an older release simply yields no storage-pool metrics).
func (c *ClusterClient) storagePools(ctx context.Context) []models.StoragePool {
 var b []byte
 if err := c.getRaw(ctx, "platform/1/storagepool/storagepools", &b); err != nil {
  log.Debugf("cluster %q: storagepools failed: %v", c.name, err)
  return nil
 }
 p, err := models.ParseStoragePools(b)
 if err != nil {
  log.Debugf("cluster %q: parse storagepools failed: %v; payload: %s", c.name, err, snippet(b))
  return nil
 }
 return p
}
```

Add `StoragePools` to the `Inventory{}` literal in `GetInventory` (after `Licenses: c.licenses(ctx),`):

```go
  StoragePools: c.storagePools(ctx),
```

Extend the debug summary log so its format string ends with `licenses=%d storage_pools=%d` and its args end with `len(inv.Licenses), len(inv.StoragePools)`. The full statement becomes:

```go
  log.Debugf("cluster %q: inventory parsed: release=%s nodes=%d (sensor values=%d) quotas=%d "+
   "nfs_exports=%d smb_shares=%d snapshots=%d sync_policies=%d events=%v licenses=%d storage_pools=%d",
   c.name, inv.Cluster.Release, len(inv.Nodes), sensors, len(inv.Quotas),
   inv.Counts.NFSExports, inv.Counts.SMBShares, inv.Counts.Snapshots,
   len(inv.SyncPolicies), inv.Events, len(inv.Licenses), len(inv.StoragePools))
```

- [ ] **Step 6: Run the e2e test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: PASS — the 9 `powerscale_storagepool_*` metrics are present.

- [ ] **Step 7: Wire the endpoint into the schema-drift guard and regenerate**

In `tools/extract-schemas/main.go`, add to the `targets` map (after the license entry):

```go
 "/platform/1/storagepool/storagepools":      "storagepools.json",
```

Then run `gofmt -w tools/extract-schemas/main.go` and regenerate:

Run: `make schemas`
Expected: `internal/powerscale/testdata/onefs_schemas.json` is updated to include the storagepools endpoint schema (git diff shows a new `storagepools.json` entry).

- [ ] **Step 8: Run the full package (schema guard included)**

Run: `go test ./internal/powerscale/`
Expected: `ok` — including `schema_guard_test.go` asserting every `storagepools.json` field (`name`, `type`, `usage.total_bytes`, `usage.used_bytes`, `usage.avail_bytes`, and the ssd/hdd variants) is documented in the 9.14 spec. If the guard fails on a field, that field is not in the v1 schema — remove it from the fixture and the parser rather than suppressing the guard.

- [ ] **Step 9: Verify gofmt-clean and commit**

Run: `gofmt -l internal/ tools/` (expect no output).

```bash
git add internal/powerscale/client.go internal/powerscale/testdata/storagepools.json internal/powerscale/testdata/onefs_schemas.json tools/extract-schemas/main.go internal/powerscale/mockserver_test.go internal/powerscale/e2e_test.go
git commit -m "feat(powerscale): best-effort storage-pool collector + schema guard + e2e coverage"
```

---

### Task 4: Documentation

**Files:**

- Modify: `docs/metrics.md` (new `## Storage pools` section)
- Modify: `docs/getting-started/configuration.md` (add `ISI_PRIV_SMARTPOOLS`)
- Modify: `docs/getting-started/installation.md` (add `ISI_PRIV_SMARTPOOLS`)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Add the Storage pools section to `docs/metrics.md`**

Insert this new section immediately before the `## Per-drive` heading (i.e. right after the `## Licenses` section's closing PromQL block):

```markdown
## Storage pools

Per-pool and per-tier capacity from SmartPools, fetched best-effort from
`storagepool/storagepools` (requires `ISI_PRIV_SMARTPOOLS`; absent if the account lacks it).
Labels: `pool` (pool/tier name) and `type` (`nodepool` or `tier`).

The list contains **both node pools and tiers**, and a tier's capacity is the sum of its
child node pools — so summing a metric across all rows double-counts. Filter
`type="nodepool"` for a non-overlapping cluster-wide total. Every metric is emitted for every
pool; an all-HDD pool simply reports `ssd=0`.

| Metric | Description |
|---|---|
| `powerscale_storagepool_total_capacity_bytes` | Total capacity of the pool/tier. |
| `powerscale_storagepool_used_capacity_bytes` | Used capacity. |
| `powerscale_storagepool_available_capacity_bytes` | Available (user-writable) capacity. |
| `powerscale_storagepool_ssd_total_capacity_bytes` | SSD-media total capacity. |
| `powerscale_storagepool_ssd_used_capacity_bytes` | SSD-media used capacity. |
| `powerscale_storagepool_ssd_available_capacity_bytes` | SSD-media available capacity. |
| `powerscale_storagepool_hdd_total_capacity_bytes` | HDD-media total capacity. |
| `powerscale_storagepool_hdd_used_capacity_bytes` | HDD-media used capacity. |
| `powerscale_storagepool_hdd_available_capacity_bytes` | HDD-media available capacity. |

Alert on a node pool over 85% full:

```promql
100 * powerscale_storagepool_used_capacity_bytes
  / powerscale_storagepool_total_capacity_bytes > 85
```

```

- [ ] **Step 2: Add `ISI_PRIV_SMARTPOOLS` to `configuration.md`**

In `docs/getting-started/configuration.md`, add this line to the privilege list (after the `ISI_PRIV_LICENSE` line). `ISI_PRIV_SMARTPOOLS` is 19 characters, so it takes two spaces before the description to match the column:

```

# ISI_PRIV_SMARTPOOLS  (storage-pool / tier capacity)

```

- [ ] **Step 3: Add `ISI_PRIV_SMARTPOOLS` to `installation.md`**

In `docs/getting-started/installation.md`, change the tail of the privilege sentence from:

```

`ISI_PRIV_SMB`, `ISI_PRIV_NFS`, and `ISI_PRIV_LICENSE`. Create a dedicated monitoring user rather than reusing

```

to:

```

`ISI_PRIV_SMB`, `ISI_PRIV_NFS`, `ISI_PRIV_LICENSE`, and `ISI_PRIV_SMARTPOOLS`. Create a dedicated monitoring user rather than reusing

```

- [ ] **Step 4: Verify the docs build**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds. (If `uvx` is unavailable, skip and note it — no Markdown change here would break the build.)

- [ ] **Step 5: Commit**

```bash
git add docs/metrics.md docs/getting-started/configuration.md docs/getting-started/installation.md
git commit -m "docs: document storage-pool metrics and ISI_PRIV_SMARTPOOLS privilege"
```

---

### Task 5: Final gate — full CI

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: gofmt clean, `go vet` clean, `golangci-lint` 0 issues, `go test -race` all packages pass (models + powerscale include the new storage-pool tests and the schema guard), `govulncheck` clean.

- [ ] **Step 2: (No commit)** — this task only gates the branch as green.

---

## Self-Review

**Spec coverage:**

- Spec "Source" (`platform/1/storagepool/storagepools`, best-effort, v1) → Task 3 Step 5. ✅
- Spec "Metrics" (9 gauges; media split in the name; always emitted) → Task 2 (`storagePoolSamples`) + Task 1 (the 9 float64 fields). ✅
- Spec "string-typed bytes via flexFloat" → Task 1 (parser uses `flexFloat`) + `TestParseStoragePools` asserts string bytes parse. ✅
- Spec "Data flow" (model+parse / client helper+Inventory / metrics label + derivations) → Tasks 1, 2, 3. ✅
- Spec "double-counting via type label" → Task 2 (`type` label) + Task 4 (docs note). ✅
- Spec "Testing" (fixture: tier + hybrid nodepool + all-HDD nodepool; schema guard; mock case; models/derivations/e2e asserts incl. all-HDD ssd=0) → Task 1 (parse test), Task 2 (build test), Task 3 (fixture, schema guard, mock, e2e). ✅
- Spec "Docs" (metrics.md section + ISI_PRIV_SMARTPOOLS) → Task 4. ✅
- Spec "Risks" (privilege name, endpoint version, string bytes) → handled by the best-effort + flexFloat design; no task needed.

**Placeholder scan:** every code/test/fixture step carries complete content; the only prose insertion point (metrics.md "before `## Per-drive`") is unambiguous. ✅

**Type consistency:** `StoragePool{Name, Type, TotalBytes, UsedBytes, AvailBytes, SSDTotalBytes, SSDUsedBytes, SSDAvailBytes, HDDTotalBytes, HDDUsedBytes, HDDAvailBytes}`, `ParseStoragePools`, `Inventory.StoragePools`, `storagePools(ctx)`, `storagePoolSamples`, `storagePoolLabels`, and the 9 metric names are used identically across Tasks 1–4. `BuildSamples(clusterName, inv, st)` called with `st=nil` in Task 2 is safe (all `st`-based builders guard `st == nil`). ✅
