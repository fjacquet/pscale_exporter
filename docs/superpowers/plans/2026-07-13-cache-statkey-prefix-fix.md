# OneFS Cache Stat-Key Prefix Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the `node.ifs.` prefix on the six OneFS cache stat keys so `statistics/current` stops 400-ing and all current-statistics metrics return, with test coverage that locks the fix in.

**Architecture:** The exporter maps OneFS stat keys → metric names via a curated JSON table (`statisticsKeys.json`), embedded and matched by exact key string in `statSamples()`. The keys were abbreviated (`cache.l1.data.read.hit`) instead of their real node-scoped form (`node.ifs.cache.l1.data.read.hit`). Because `statistics/current` is an all-or-nothing batch, one invalid key drops every current-statistics metric. Fix = correct the six strings; no code logic changes. Metric names are unchanged this pass (units are trace-validated later).

**Tech Stack:** Go, `go test`, testify-free table tests, `httptest` mock OneFS server, Prometheus client_golang, Makefile CI.

## Global Constraints

Copied verbatim from the spec and project conventions — every task implicitly includes these:

- **Keep the `powerscale_` metric prefix** (matches `dell/csm-metrics-powerscale` for dashboard compatibility).
- **Metric names carry their unit** (`_bytes`, `_bytes_per_second`, `_operations_per_second`, `_microseconds`, `_percent`).
- **`iops` / bandwidth are per-second gauges** — aggregate in PromQL with `sum`/`avg`, never `rate()`. (A cache key confirmed as a *counter* is the exception and would be renamed off `_per_second`.)
- **Node-scope keys map `devid` → node LNN**; `scope` stays `node`.
- **Semgrep write-hook blocks on findings and inline `// nosemgrep` is NOT honored** — fix by restructuring. Test HTTP handlers must write fixtures through the `writeBytes(io.Writer, …)` helper (already in `mockserver_test.go`), never directly to a `ResponseWriter`.
- **Do not enable the SDK's verbose logging** (`GOISILON_DEBUG` / `verboseLogging`) — it leaks `Set-Cookie: isisessid=…`. Units validation uses the exporter's own `--trace` (body only, never headers).
- **Branch:** work happens on `fix/onefs-cache-statkey-prefix` (already created; the spec is committed there).

---

### Task 1: Correct the six cache key strings

**Files:**
- Modify: `internal/powerscale/statisticsKeys.json:18-23`
- Test: `internal/powerscale/derivations_test.go` (add one test function)

**Interfaces:**
- Consumes: `BuildSamples(clusterName string, inv *models.Inventory, st *models.Statistics) []Sample`, `models.Inventory`, `models.ClusterInfo`, `models.Node{ID,LNN}`, `models.Statistics{Current []models.StatPoint}`, `models.StatPoint{Key string, DevID int, Value float64}`, `Sample{Name string, Labels []Label, Value float64}` — all already defined in the `powerscale`/`models` packages.
- Produces: nothing new; asserts existing behavior against corrected keys.

- [ ] **Step 1: Write the failing test**

Add to `internal/powerscale/derivations_test.go` (after `TestBuildSamplesClusterAndNode`):

```go
func TestBuildSamplesNodeIfsCacheKeys(t *testing.T) {
	inv := &models.Inventory{
		Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
		Nodes:   []models.Node{{ID: 1, LNN: 1}},
	}
	st := &models.Statistics{
		Current: []models.StatPoint{
			{Key: "node.ifs.cache.l1.data.read.hit", DevID: 1, Value: 1000},
			{Key: "node.ifs.cache.l1.data.read.miss", DevID: 1, Value: 100},
			{Key: "node.ifs.cache.l2.data.read.hit", DevID: 1, Value: 2000},
			{Key: "node.ifs.cache.l2.data.read.miss", DevID: 1, Value: 200},
			{Key: "node.ifs.cache.l3.data.read.hit", DevID: 1, Value: 3000},
			{Key: "node.ifs.cache.l3.data.read.miss", DevID: 1, Value: 300},
		},
	}
	samples := BuildSamples("clu1", inv, st)
	get := func(name string) (Sample, bool) {
		for _, s := range samples {
			if s.Name == name {
				return s, true
			}
		}
		return Sample{}, false
	}
	cases := []struct {
		metric string
		value  float64
	}{
		{"powerscale_node_cache_l1_read_hit_bytes_per_second", 1000},
		{"powerscale_node_cache_l1_read_miss_bytes_per_second", 100},
		{"powerscale_node_cache_l2_read_hit_bytes_per_second", 2000},
		{"powerscale_node_cache_l2_read_miss_bytes_per_second", 200},
		{"powerscale_node_cache_l3_read_hit_bytes_per_second", 3000},
		{"powerscale_node_cache_l3_read_miss_bytes_per_second", 300},
	}
	for _, c := range cases {
		s, ok := get(c.metric)
		if !ok || s.Value != c.value {
			t.Fatalf("cache sample %s wrong: %+v ok=%v", c.metric, s, ok)
		}
		if s.Labels[2].Value != "1" { // nodeLabels = [cluster, cluster_id, node]
			t.Fatalf("cache sample %s node label wrong: %+v", c.metric, s.Labels)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesNodeIfsCacheKeys -v`
Expected: FAIL — `statKeyByKey` still holds `cache.l1.data.read.hit` (no `node.ifs.` prefix), so the correct keys don't match and no cache samples are produced (`ok=false`).

- [ ] **Step 3: Fix the key strings**

In `internal/powerscale/statisticsKeys.json`, replace lines 18-23 (only the `key` field changes; `metric` and `scope` stay):

```json
  {"key": "node.ifs.cache.l1.data.read.hit",  "metric": "powerscale_node_cache_l1_read_hit_bytes_per_second",  "scope": "node"},
  {"key": "node.ifs.cache.l1.data.read.miss", "metric": "powerscale_node_cache_l1_read_miss_bytes_per_second", "scope": "node"},
  {"key": "node.ifs.cache.l2.data.read.hit",  "metric": "powerscale_node_cache_l2_read_hit_bytes_per_second",  "scope": "node"},
  {"key": "node.ifs.cache.l2.data.read.miss", "metric": "powerscale_node_cache_l2_read_miss_bytes_per_second", "scope": "node"},
  {"key": "node.ifs.cache.l3.data.read.hit",  "metric": "powerscale_node_cache_l3_read_hit_bytes_per_second",  "scope": "node"},
  {"key": "node.ifs.cache.l3.data.read.miss", "metric": "powerscale_node_cache_l3_read_miss_bytes_per_second", "scope": "node"}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesNodeIfsCacheKeys -v`
Expected: PASS.

- [ ] **Step 5: Run the full package to confirm no regressions**

Run: `go test ./internal/powerscale/`
Expected: `ok` (all existing tests still pass; the schema-drift guard is unaffected — key strings are data values, not schema fields).

- [ ] **Step 6: Commit**

```bash
git add internal/powerscale/statisticsKeys.json internal/powerscale/derivations_test.go
git commit -m "fix(powerscale): restore node.ifs. prefix on cache stat keys"
```

---

### Task 2: End-to-end fixture coverage

**Files:**
- Modify: `internal/powerscale/testdata/stat_current.json`
- Modify: `internal/powerscale/e2e_test.go:41-58` (the `want` presence map)

**Interfaces:**
- Consumes: the corrected keys from Task 1 (`statKeyByKey` now matches `node.ifs.cache.*`). The mock server serves the whole `stat_current.json` file for any `.../statistics/current` request (`mockserver_test.go:64-65`), so fixture rows are surfaced without key filtering. `nodes.json` maps `devid 1 → lnn 1`.
- Produces: nothing new; end-to-end proof that cache metrics reach the Prometheus registry.

- [ ] **Step 1: Add the cache metrics to the e2e presence map (failing first)**

In `internal/powerscale/e2e_test.go`, add these six entries inside the `want := map[string]bool{ … }` block (e.g. after `"powerscale_node_temperature_celsius": false,`):

```go
		"powerscale_node_cache_l1_read_hit_bytes_per_second":  false,
		"powerscale_node_cache_l1_read_miss_bytes_per_second": false,
		"powerscale_node_cache_l2_read_hit_bytes_per_second":  false,
		"powerscale_node_cache_l2_read_miss_bytes_per_second": false,
		"powerscale_node_cache_l3_read_hit_bytes_per_second":  false,
		"powerscale_node_cache_l3_read_miss_bytes_per_second": false,
```

- [ ] **Step 2: Run the e2e test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: FAIL — `missing metric powerscale_node_cache_l1_read_hit_bytes_per_second` (and the other five): the fixture has no cache rows yet, so nothing is emitted.

- [ ] **Step 3: Add cache rows to the fixture**

In `internal/powerscale/testdata/stat_current.json`, add six rows inside the `"stats"` array (keep the existing four; mind the trailing comma on the previous last row):

```json
{"devid": 1, "error": null, "key": "node.ifs.cache.l1.data.read.hit",  "time": 1700000000, "value": 1000},
{"devid": 1, "error": null, "key": "node.ifs.cache.l1.data.read.miss", "time": 1700000000, "value": 100},
{"devid": 1, "error": null, "key": "node.ifs.cache.l2.data.read.hit",  "time": 1700000000, "value": 2000},
{"devid": 1, "error": null, "key": "node.ifs.cache.l2.data.read.miss", "time": 1700000000, "value": 200},
{"devid": 1, "error": null, "key": "node.ifs.cache.l3.data.read.hit",  "time": 1700000000, "value": 3000},
{"devid": 1, "error": null, "key": "node.ifs.cache.l3.data.read.miss", "time": 1700000000, "value": 300}
```

The resulting file (for reference):

```json
{"stats": [
  {"devid": 0, "error": null, "key": "ifs.bytes.total", "time": 1700000000, "value": 5000},
  {"devid": 0, "error": null, "key": "ifs.bytes.used",  "time": 1700000000, "value": 2000},
  {"devid": 0, "error": null, "key": "cluster.cpu.sys.avg", "time": 1700000000, "value": 12.5},
  {"devid": 2, "error": null, "key": "node.memory.used", "time": 1700000000, "value": 42},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l1.data.read.hit",  "time": 1700000000, "value": 1000},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l1.data.read.miss", "time": 1700000000, "value": 100},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l2.data.read.hit",  "time": 1700000000, "value": 2000},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l2.data.read.miss", "time": 1700000000, "value": 200},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l3.data.read.hit",  "time": 1700000000, "value": 3000},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l3.data.read.miss", "time": 1700000000, "value": 300}
]}
```

- [ ] **Step 4: Run the e2e test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: PASS — all six cache metrics are now present.

- [ ] **Step 5: Run the full package**

Run: `go test ./internal/powerscale/`
Expected: `ok` (schema-drift guard still green — the new rows use the documented `devid/error/key/time/value` fields).

- [ ] **Step 6: Commit**

```bash
git add internal/powerscale/testdata/stat_current.json internal/powerscale/e2e_test.go
git commit -m "test(powerscale): cover node.ifs.cache.* keys end-to-end"
```

---

### Task 3: Correct the docs caveat

**Files:**
- Modify: `docs/metrics.md:55-66` (the `### Cache (provisional)` section)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Replace the Cache section**

In `docs/metrics.md`, replace the block currently spanning lines 55-66 with:

```markdown
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
```

- [ ] **Step 2: Verify the docs build (strict)**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds with no warnings. (If `uvx` is unavailable in the environment, skip and note it; there is no Markdown syntax change that would break the build.)

- [ ] **Step 3: Commit**

```bash
git add docs/metrics.md
git commit -m "docs(metrics): correct cache stat-key caveat (all-or-nothing batch, node.ifs.)"
```

---

### Task 4: Correct the project-memory note

**Files:**
- Modify: `/Users/fjacquet/.claude/projects/-Users-fjacquet-Projects-pscale-exporter/memory/provisional-onefs-keys.md`

**Interfaces:** none. This is Claude's auto-memory, **outside the repo — no git commit applies.** Included because the spec (§4) requires the wrong failure-mode claim be corrected so future sessions don't repeat it.

- [ ] **Step 1: Fix the "silently skipped" claim**

In `provisional-onefs-keys.md`, in the first "Cache keys" bullet, replace the sentence:

> Unknown keys are silently skipped (ParseStatCurrent drops rows with `error != null`), so a wrong key degrades to an empty series, not an error.

with:

> A batched `statistics/current` request is all-or-nothing: OneFS 400s the *whole* call on the first invalid key, so a wrong key drops **all** current-statistics metrics for that cluster, not just its own series. (The per-row `error != null` skip in ParseStatCurrent only applies to rows OneFS *does* return.)

- [ ] **Step 2: Update the "STILL UNVALIDATED" section**

Replace the final "**STILL UNVALIDATED**" paragraph with:

> **Cache key STRINGS confirmed 2026-07-13** against a live cluster: `isi statistics list keys` shows the real keys are node-scoped `node.ifs.cache.l1/l2/l3.data.read.hit|miss` — the exporter had dropped the `node.ifs.` prefix. Fixed on branch `fix/onefs-cache-statkey-prefix`. **STILL PENDING:** the unit semantics (per-second rate vs cumulative counter) of these keys — resolve via `--once --trace` against a live cluster and adjust the `_bytes_per_second` suffix if they are counters (see design spec §3).

- [ ] **Step 3: No commit**

This file is outside the repository; do not run `git add`/`git commit` for it.

---

### Task 5: Final gate — full test + CI

**Files:** none (verification only).

**Interfaces:** none.

- [ ] **Step 1: Run the race+coverage tests**

Run: `make test-race`
Expected: all packages `ok`, no data races.

- [ ] **Step 2: Run the full CI gate**

Run: `make ci`
Expected: gofmt clean, `go vet` clean, `golangci-lint` clean, `go test -race` pass, `govulncheck` clean.

- [ ] **Step 3: (No commit)** — nothing changed; this task only gates the branch as green.

---

## Deferred — live-cluster units validation (needs the box)

Not part of the code branch; run when a cluster is reachable (spec §3):

1. Build: `make cli`.
2. Run `./bin/pscale_exporter --config config.yaml --once --debug` → confirm no `Invalid key` error and that the six `powerscale_node_cache_*` samples appear.
3. Run `./bin/pscale_exporter --config config.yaml --trace` (looping) and read the raw `platform/1/statistics/current` body across ≥2 intervals:
   - **monotonically increasing** bytes → **counter** → rename metrics to `powerscale_node_cache_lN_read_{hit,miss}_bytes`, update the fixture/test names, and change the docs PromQL hint to `rate(...)`; hit ratio = `sum(rate(hit)) / sum(rate(hit) + rate(miss))`.
   - **stationary** bytes/sec → **rate** → keep `_bytes_per_second`; drop the "(provisional)" markers in `docs/metrics.md` and the "STILL PENDING" note in memory.
4. `GET /platform/1/statistics/keys/node.ifs.cache.l1.data.read.hit` (`units`/`type`) settles it exactly if API access is available.

---

## Self-Review

**Spec coverage:**
- Spec §1 (key-string fix) → Task 1. ✅
- Spec §2 (test coverage) → Task 1 (unit) + Task 2 (e2e fixture + presence map). ✅
- Spec §3 (units, trace-gated) → names kept as-is in Tasks 1-2; procedure captured in "Deferred" + docs/memory notes. ✅
- Spec §4 (docs) → Task 3; (memory) → Task 4. ✅
- Spec §5 (optional hardening) → intentionally omitted (spec marks it "comment only, if at all"; YAGNI). Noted here so the omission is deliberate.
- Spec "Verification plan" → Task 5 (`make test-race`, `make ci`) + Deferred (live `--once --debug`/`--trace`). ✅

**Placeholder scan:** No TBD/TODO; every code step shows complete content; the fixture full-file reference removes ambiguity. ✅

**Type consistency:** `BuildSamples`, `models.StatPoint{Key,DevID,Value}`, `Sample{Name,Labels,Value}`, `Labels[2]` = node label, and the six metric names are used identically across Task 1, Task 2, and Task 3. ✅
