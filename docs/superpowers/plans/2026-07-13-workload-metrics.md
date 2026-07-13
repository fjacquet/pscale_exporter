# OneFS Per-Workload Performance Metrics (#32) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose per-workload performance (`powerscale_workload_{operations_per_second,in_bytes_per_second,out_bytes_per_second,cpu_microseconds}`) so operators can attribute cluster load to a zone / protocol / user / job.

**Architecture:** The established best-effort typed-collector pattern, on the *statistics* side (like the drive/client summaries): a `models.Workload` type + `ParseWorkloadSummary`, a best-effort `c.workloadSummary(ctx)` fetch stored on the `Statistics` struct in `GetStatistics`, and a `workloadSamples` builder in `derivations.go`. No new architecture.

**Tech Stack:** Go, `go test`, `httptest` mock OneFS server, the CycloneDX schema-drift guard (`make schemas`), Prometheus client_golang.

## Global Constraints

- Keep the `powerscale_` metric prefix; metric names are unit/meaning-explicit.
- Canonical leading labels on every sample: `cluster`, then `cluster_id` (via `baseLabels`), then the fixed workload label set `node`, `zone`, `protocol`, `username`, `system_name`, `job_type`.
- **Fixed label set:** all four workload metrics carry the SAME 8 label names; an unpinned dimension is the empty string `""` (Prometheus requires a consistent label-name set per metric name). Never conditionally omit a label.
- **All four gauges are per-second rates** — emitted for every workload row, always. `cpu` is µs-of-CPU-per-second (a rate, not a counter).
- **Best-effort collector:** a fetch/parse error logs at debug and returns `nil` — it never fails statistics collection (same as `driveSummary`, `client.go`).
- **Perf fields are JSON numbers** (not strings) → plain `float64`, no `flexFloat`. `node` is a JSON number decoded via `float64`→`int`. Nullable identity strings decode to `""` (Go's `encoding/json` leaves a string field unchanged — zero value `""` — on JSON `null`).
- **No `devid`→LNN mapping** for `node`: workload rows report `node` directly (`0` = cluster-scoped).
- Fixtures are validated against the OneFS 9.14 OpenAPI spec by the schema-drift guard (`make schemas`); every fixture field must be documented in the spec.
- **No new privilege:** `ISI_PRIV_STATISTICS` already covers this endpoint (do not touch the privilege docs).
- A Semgrep hook scans files on write and **blocks on findings**; inline `// nosemgrep` is not honored. Test HTTP handlers must write through the `writeBytes(io.Writer, …)` helper (already in `mockserver_test.go`), never directly to a `ResponseWriter`.
- Branch: `feat/workload-metrics` (already created; stacked on `feat/storagepool-metrics`; the spec is committed there).

---

### Task 1: Workload model + `ParseWorkloadSummary`

**Files:**
- Modify: `internal/models/onefs.go` (add `Workload` type, `ParseWorkloadSummary`, `Statistics.Workloads` field)
- Test: `internal/models/onefs_test.go`

**Interfaces:**
- Consumes: `encoding/json` (already imported in `onefs.go`).
- Produces: `type Workload struct { Node int; Zone, Protocol, Username, SystemName, JobType string; Ops, BytesIn, BytesOut, CPUMicros float64 }`; `func ParseWorkloadSummary(b []byte) ([]Workload, error)`; `Statistics.Workloads []Workload`.

- [ ] **Step 1: Write the failing test**

Add to `internal/models/onefs_test.go` (place after `TestParseClientSummary` or near the other summary-parse tests):

```go
func TestParseWorkloadSummary(t *testing.T) {
	data := []byte(`{"workload":[
		{"node":1,"zone_name":"System","protocol":"nfs3","username":"alice","system_name":null,"job_type":null,"ops":120.5,"bytes_in":1024,"bytes_out":2048,"cpu":50000},
		{"node":0,"zone_name":null,"protocol":null,"username":null,"system_name":null,"job_type":null,"ops":5,"bytes_in":10,"bytes_out":20,"cpu":100}
	]}`)
	ws, err := ParseWorkloadSummary(data)
	if err != nil || len(ws) != 2 {
		t.Fatalf("parse: %d err=%v", len(ws), err)
	}
	if ws[0] != (Workload{Node: 1, Zone: "System", Protocol: "nfs3", Username: "alice", Ops: 120.5, BytesIn: 1024, BytesOut: 2048, CPUMicros: 50000}) {
		t.Fatalf("workload[0]: %+v", ws[0])
	}
	if ws[1].Node != 0 || ws[1].Zone != "" || ws[1].Protocol != "" {
		t.Fatalf("workload[1] (aggregate, null dims -> empty string): %+v", ws[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestParseWorkloadSummary -v`
Expected: FAIL — `undefined: ParseWorkloadSummary` / `undefined: Workload`.

- [ ] **Step 3: Implement the model + parser**

In `internal/models/onefs.go`, add the field to the `Statistics` struct (after `Clients []ClientStat`):

```go
	Workloads []Workload
```

And add the type + parser (place near `ParseClientSummary`):

```go
// Workload is one per-workload performance row (statistics/summary/workload). The identity
// dimensions are populated per the cluster's OneFS performance-dataset definition; an
// unpinned dimension is the empty string. All perf fields are per-second rates (CPUMicros is
// microseconds of CPU per second across all cores).
type Workload struct {
	Node       int
	Zone       string
	Protocol   string
	Username   string
	SystemName string
	JobType    string
	Ops        float64
	BytesIn    float64
	BytesOut   float64
	CPUMicros  float64
}

// ParseWorkloadSummary parses platform/N/statistics/summary/workload. Perf fields are JSON
// numbers; nullable identity strings decode to "" (encoding/json leaves a string field
// unchanged on JSON null). node is a JSON number decoded via float64 then truncated to int,
// so a "1.0"-style value cannot fail the parse.
func ParseWorkloadSummary(b []byte) ([]Workload, error) {
	var raw struct {
		Workload []struct {
			Node       float64 `json:"node"`
			ZoneName   string  `json:"zone_name"`
			Protocol   string  `json:"protocol"`
			Username   string  `json:"username"`
			SystemName string  `json:"system_name"`
			JobType    string  `json:"job_type"`
			Ops        float64 `json:"ops"`
			BytesIn    float64 `json:"bytes_in"`
			BytesOut   float64 `json:"bytes_out"`
			CPU        float64 `json:"cpu"`
		} `json:"workload"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]Workload, 0, len(raw.Workload))
	for _, w := range raw.Workload {
		out = append(out, Workload{
			Node:       int(w.Node),
			Zone:       w.ZoneName,
			Protocol:   w.Protocol,
			Username:   w.Username,
			SystemName: w.SystemName,
			JobType:    w.JobType,
			Ops:        w.Ops,
			BytesIn:    w.BytesIn,
			BytesOut:   w.BytesOut,
			CPUMicros:  w.CPU,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -run TestParseWorkloadSummary -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/onefs.go internal/models/onefs_test.go
git commit -m "feat(models): Workload type and ParseWorkloadSummary"
```

---

### Task 2: `workloadSamples` builder + label helper

**Files:**
- Modify: `internal/powerscale/metrics.go` (add `workloadLabels`)
- Modify: `internal/powerscale/derivations.go` (add `workloadSamples`, wire into `BuildSamples`)
- Test: `internal/powerscale/derivations_test.go`

**Interfaces:**
- Consumes: `models.Workload`, `Statistics.Workloads` (Task 1); `baseLabels`, `Sample`, `Label`, `strconv` (existing).
- Produces: `workloadLabels(clusterName, clusterID, node, zone, protocol, username, systemName, jobType string) []Label`; `workloadSamples(clusterName, clusterID string, st *models.Statistics) []Sample`; emits the 4 metric names `powerscale_workload_{operations_per_second,in_bytes_per_second,out_bytes_per_second,cpu_microseconds}`.

- [ ] **Step 1: Write the failing test**

Add to `internal/powerscale/derivations_test.go` (place after `TestBuildSamplesStoragePools`):

```go
func TestBuildSamplesWorkloads(t *testing.T) {
	inv := &models.Inventory{Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"}}
	st := &models.Statistics{
		Workloads: []models.Workload{
			{Node: 1, Zone: "System", Protocol: "nfs3", Username: "alice", Ops: 120, BytesIn: 1024, BytesOut: 2048, CPUMicros: 50000},
			{Node: 0, Ops: 5, BytesIn: 10, BytesOut: 20, CPUMicros: 100}, // aggregate: all dims empty
		},
	}
	samples := BuildSamples("clu1", inv, st)
	find := func(name, username string) (Sample, bool) {
		for _, s := range samples {
			if s.Name != name {
				continue
			}
			for _, l := range s.Labels {
				if l.Name == "username" && l.Value == username {
					return s, true
				}
			}
		}
		return Sample{}, false
	}
	if s, ok := find("powerscale_workload_operations_per_second", "alice"); !ok || s.Value != 120 {
		t.Fatalf("alice ops wrong: %+v ok=%v", s, ok)
	}
	if s, ok := find("powerscale_workload_cpu_microseconds", "alice"); !ok || s.Value != 50000 {
		t.Fatalf("alice cpu wrong: %+v ok=%v", s, ok)
	}
	// aggregate row: username="" and zone="" but still emits values (empty-label path)
	s, ok := find("powerscale_workload_in_bytes_per_second", "")
	if !ok || s.Value != 10 {
		t.Fatalf("aggregate in_bytes wrong: %+v ok=%v", s, ok)
	}
	hasEmptyZone := false
	for _, l := range s.Labels {
		if l.Name == "zone" && l.Value == "" {
			hasEmptyZone = true
		}
	}
	if !hasEmptyZone {
		t.Fatalf("aggregate row should carry an empty zone label: %+v", s.Labels)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesWorkloads -v`
Expected: FAIL — `workloadSamples` not wired / metrics absent.

- [ ] **Step 3: Add the label helper**

In `internal/powerscale/metrics.go` (after `storagePoolLabels`):

```go
// workloadLabels appends the curated per-workload identity dimensions. Any dimension not
// pinned by the cluster's performance dataset is the empty string, keeping the label-name
// set consistent across rows.
func workloadLabels(clusterName, clusterID, node, zone, protocol, username, systemName, jobType string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "node", Value: node},
		Label{Name: "zone", Value: zone},
		Label{Name: "protocol", Value: protocol},
		Label{Name: "username", Value: username},
		Label{Name: "system_name", Value: systemName},
		Label{Name: "job_type", Value: jobType},
	)
}
```

- [ ] **Step 4: Add `workloadSamples` and wire it in**

In `internal/powerscale/derivations.go`, add the builder (place after `clientSamples`):

```go
// workloadSamples emits per-workload performance (ops, throughput, CPU). Rows come from OneFS
// performance datasets; the identity dimensions are labels (unpinned ones are ""). All four
// gauges are per-second rates — aggregate with sum/avg, never rate().
func workloadSamples(clusterName, clusterID string, st *models.Statistics) []Sample {
	if st == nil {
		return nil
	}
	var out []Sample
	for _, w := range st.Workloads {
		labels := workloadLabels(clusterName, clusterID, strconv.Itoa(w.Node), w.Zone, w.Protocol, w.Username, w.SystemName, w.JobType)
		out = append(out,
			Sample{Name: "powerscale_workload_operations_per_second", Labels: labels, Value: w.Ops},
			Sample{Name: "powerscale_workload_in_bytes_per_second", Labels: labels, Value: w.BytesIn},
			Sample{Name: "powerscale_workload_out_bytes_per_second", Labels: labels, Value: w.BytesOut},
			Sample{Name: "powerscale_workload_cpu_microseconds", Labels: labels, Value: w.CPUMicros},
		)
	}
	return out
}
```

Wire it into `BuildSamples` (add immediately after the `clientSamples(...)` append line, which is currently the last append before `return samples`):

```go
	samples = append(samples, workloadSamples(clusterName, clusterID, st)...)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesWorkloads -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/powerscale/metrics.go internal/powerscale/derivations.go internal/powerscale/derivations_test.go
git commit -m "feat(powerscale): workloadSamples builder for per-workload metrics"
```

---

### Task 3: Best-effort fetch, fixture, schema guard, and e2e coverage

**Files:**
- Modify: `internal/powerscale/client.go` (add `workloadSummary` helper + `GetStatistics` wiring + debug log)
- Create: `internal/powerscale/testdata/stat_workload.json`
- Modify: `tools/extract-schemas/main.go` (targets map) — then regenerate via `make schemas`
- Modify: `internal/powerscale/mockserver_test.go` (serve the fixture)
- Modify: `internal/powerscale/e2e_test.go` (presence map)

**Interfaces:**
- Consumes: `models.ParseWorkloadSummary`, `Statistics.Workloads` (Task 1); the emitted metric names (Task 2); `c.getRaw`, `snippet`, `log` (existing).
- Produces: `func (c *ClusterClient) workloadSummary(ctx context.Context) []models.Workload`; the `stat_workload.json` fixture; end-to-end emission of the 4 workload metrics.

- [ ] **Step 1: Add the 4 workload metrics to the e2e presence map (failing first)**

In `internal/powerscale/e2e_test.go`, add to the `want := map[string]bool{ … }` block (after the last `powerscale_storagepool_*` line):

```go
		"powerscale_workload_operations_per_second": false,
		"powerscale_workload_in_bytes_per_second":   false,
		"powerscale_workload_out_bytes_per_second":  false,
		"powerscale_workload_cpu_microseconds":      false,
```

Then run `gofmt -w internal/powerscale/e2e_test.go` to fix column alignment.

- [ ] **Step 2: Run the e2e test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: FAIL — `missing metric powerscale_workload_*` (nothing fetches/serves workloads yet).

- [ ] **Step 3: Add the fixture**

Create `internal/powerscale/testdata/stat_workload.json`. Row 1 is a dataset-pinned workload (zone/protocol/username populated); row 2 is the cluster aggregate (node 0, all string dims `null`) exercising the empty-label path:

```json
{"workload": [
  {"node": 1, "zone_name": "System", "protocol": "nfs3", "username": "alice", "system_name": null, "job_type": null, "ops": 120.5, "bytes_in": 1048576, "bytes_out": 2097152, "cpu": 50000},
  {"node": 0, "zone_name": null, "protocol": null, "username": null, "system_name": null, "job_type": null, "ops": 5, "bytes_in": 4096, "bytes_out": 8192, "cpu": 100}
]}
```

- [ ] **Step 4: Serve the fixture from the mock server**

In `internal/powerscale/mockserver_test.go`, add a case to the path switch (before `default`), mirroring the existing statistics-summary cases:

```go
		case strings.HasSuffix(p, "/statistics/summary/workload"):
			writeBytes(w, fixture(t, "stat_workload.json"))
```

- [ ] **Step 5: Add the best-effort fetch + wire into `GetStatistics`**

In `internal/powerscale/client.go`, add the helper (place after `clientSummary`):

```go
// workloadSummary fetches per-workload performance best-effort. Rows require OneFS
// performance datasets (isi performance datasets) to be configured; without one this yields
// few or no rows.
func (c *ClusterClient) workloadSummary(ctx context.Context) []models.Workload {
	var b []byte
	if err := c.getRaw(ctx, "platform/4/statistics/summary/workload", &b); err != nil {
		log.Debugf("cluster %q: workload summary failed: %v", c.name, err)
		return nil
	}
	w, err := models.ParseWorkloadSummary(b)
	if err != nil {
		log.Debugf("cluster %q: parse workload summary failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return w
}
```

In `GetStatistics`, set `st.Workloads` alongside the existing `st.Drives`/`st.Clients` assignments (they read `st.Drives = c.driveSummary(ctx)` / `st.Clients = c.clientSummary(ctx)`); add:

```go
	st.Workloads = c.workloadSummary(ctx)
```

Extend the `GetStatistics` debug summary log so its format string ends with ` workload_rows=%d` and its args end with `len(st.Workloads)`. The full statement becomes:

```go
		log.Debugf("cluster %q: statistics parsed: keys=%d/%d requested (missing: %v) "+
			"proto_rows=%d drive_rows=%d client_rows=%d workload_rows=%d",
			c.name, len(returned), len(keys), missing, len(st.Proto), len(st.Drives), len(st.Clients), len(st.Workloads))
```

- [ ] **Step 6: Run the e2e test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: PASS — the 4 `powerscale_workload_*` metrics are present.

- [ ] **Step 7: Wire the endpoint into the schema-drift guard and regenerate**

In `tools/extract-schemas/main.go`, add to the `targets` map (after the storagepool entry):

```go
	"/platform/4/statistics/summary/workload":   "stat_workload.json",
```

Then run `gofmt -w tools/extract-schemas/main.go` and regenerate:

Run: `make schemas`
Expected: `internal/powerscale/testdata/onefs_schemas.json` is updated to include a `stat_workload.json` entry with dotted `workload.*` fields (git diff shows it).

- [ ] **Step 8: Run the full package (schema guard included)**

Run: `go test ./internal/powerscale/`
Expected: `ok` — including `schema_guard_test.go` asserting every `stat_workload.json` field (`workload.node`, `workload.zone_name`, `workload.protocol`, `workload.username`, `workload.system_name`, `workload.job_type`, `workload.ops`, `workload.bytes_in`, `workload.bytes_out`, `workload.cpu`) is documented in the 9.14 v4 schema. If the guard fails on a field, that field is not in the v4 schema — remove it from the fixture and the parser rather than suppressing the guard.

- [ ] **Step 9: Verify gofmt-clean and commit**

Run: `gofmt -l internal/ tools/` (expect no output).

```bash
git add internal/powerscale/client.go internal/powerscale/testdata/stat_workload.json internal/powerscale/testdata/onefs_schemas.json tools/extract-schemas/main.go internal/powerscale/mockserver_test.go internal/powerscale/e2e_test.go
git commit -m "feat(powerscale): best-effort workload collector + schema guard + e2e coverage"
```

---

### Task 4: Documentation

**Files:**
- Modify: `docs/metrics.md` (new `## Workloads` section)

**Interfaces:** none (documentation only). No privilege-doc change — `ISI_PRIV_STATISTICS` is already documented.

- [ ] **Step 1: Add the Workloads section to `docs/metrics.md`**

Insert this new section immediately before the `## Health & metadata` heading (i.e. right after the `## Per-client` section's table):

```markdown
## Workloads

Per-workload performance, from `statistics/summary/workload`. Best-effort and covered by
`ISI_PRIV_STATISTICS` (already required for the other statistics).

!!! note "Requires a configured performance dataset"
    Workload rows are produced by OneFS **performance datasets** (`isi performance datasets`).
    Without a configured dataset this endpoint returns few or only aggregate rows, so these
    metrics may be empty until you define one. The dataset's pinned dimensions also determine
    which labels are populated — an unpinned dimension is the empty string.

Labels: `node` (`0` = cluster-scoped), `zone`, `protocol`, `username`, `system_name`,
`job_type`. The high-cardinality dimensions OneFS also reports (`path`, client IP addresses,
SIDs) are intentionally **not** exported; control cardinality through your dataset definition.

| Metric | Unit | Description |
|---|---|---|
| `powerscale_workload_operations_per_second` | ops/s | Operation rate for the workload. |
| `powerscale_workload_in_bytes_per_second` | bytes/s | Inbound throughput. |
| `powerscale_workload_out_bytes_per_second` | bytes/s | Outbound throughput. |
| `powerscale_workload_cpu_microseconds` | µs/s | CPU time consumed per second across all cores (a per-second gauge — `sum`/`avg`, never `rate()`). |

Top 5 workloads by operation rate, per cluster:

```promql
topk(5, sum by (cluster, zone, username, protocol) (powerscale_workload_operations_per_second))
```

Per-workload disk/cache detail (`reads`, `writes`, `l2`, `l3`) and **latency** are planned
follow-ups — latency is held back until a live workload body confirms its unit.
```

- [ ] **Step 2: Verify the docs build**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds (pre-existing "not in nav" INFO lines about superpowers/ docs are normal). If `uvx` is unavailable, skip and note it — no Markdown change here would break the build.

- [ ] **Step 3: Commit**

```bash
git add docs/metrics.md
git commit -m "docs: document per-workload metrics and the performance-dataset prerequisite"
```

---

### Task 5: Final gate — full CI

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: gofmt clean, `go vet` clean, `golangci-lint` 0 issues, `go test -race` all packages pass (models + powerscale include the new workload tests and the schema guard), `govulncheck` clean.

- [ ] **Step 2: (No commit)** — this task only gates the branch as green.

---

## Self-Review

**Spec coverage:**
- Spec "Source" (`platform/4/statistics/summary/workload`, best-effort, on the `Statistics` struct, v4) → Task 1 (`Statistics.Workloads`) + Task 3 Step 5. ✅
- Spec "Metrics" (4 gauges, per-second, `cpu` µs/s, always emitted) → Task 2 (`workloadSamples`) + Task 1 (4 float64 fields). ✅
- Spec "Labels" (fixed 8-label set; unpinned → ""; `node` direct, no devid→LNN) → Task 2 (`workloadLabels` + `strconv.Itoa(w.Node)`) + Task 1 (null→"" via json). ✅
- Spec "perf fields JSON numbers; node float64→int; nullable strings→''" → Task 1 (parser) + `TestParseWorkloadSummary` (asserts null→"" and node 0/1). ✅
- Spec "Data flow" (model+parse / client helper+GetStatistics / metrics label + derivations) → Tasks 1, 2, 3. ✅
- Spec "Testing" (fixture: pinned row + aggregate node-0 empty-dims row; schema guard; mock; models/derivations/e2e incl. empty-label path) → Task 1 (parse test), Task 2 (build test w/ empty-label assert), Task 3 (fixture, schema guard, mock, e2e). ✅
- Spec "Docs" (metrics.md section + prerequisite + cardinality + latency-follow-up; NO privilege change) → Task 4. ✅
- Spec "Non-goals / Risks" (latency deferred; reads/writes/l2/l3 out; no cap; cpu unit note) → handled by scope + docs; no task needed.

**Placeholder scan:** every code/test/fixture step carries complete content; the one prose insertion point (metrics.md "before `## Health & metadata`") is unambiguous. ✅

**Type consistency:** `Workload{Node, Zone, Protocol, Username, SystemName, JobType, Ops, BytesIn, BytesOut, CPUMicros}`, `ParseWorkloadSummary`, `Statistics.Workloads`, `workloadSummary(ctx)`, `workloadSamples`, `workloadLabels`, and the 4 metric names are used identically across Tasks 1–4. `workloadSamples` guards `st == nil` like the other `st`-based builders; `BuildSamples` passes `st` through. ✅
