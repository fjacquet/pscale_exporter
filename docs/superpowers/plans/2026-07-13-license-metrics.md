# OneFS License Metrics (#34) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose per-feature OneFS license status and expiry (`powerscale_license_days_to_expiry`, `_expired`, `_info`) so operators can alert before a licensed feature lapses.

**Architecture:** The established best-effort typed-collector pattern — a `models.License` type + `ParseLicenses`, a best-effort `c.licenses(ctx)` fetch wired into `GetInventory`, and a `licenseSamples` builder in `derivations.go`. No new architecture; mirrors `syncPolicies`/`syncSamples`.

**Tech Stack:** Go, `go test`, `httptest` mock OneFS server, the CycloneDX schema-drift guard (`make schemas`), Prometheus client_golang.

## Global Constraints

- Keep the `powerscale_` metric prefix; metric names are unit/meaning-explicit.
- Canonical leading labels on every sample: `cluster`, then `cluster_id` (via `baseLabels`).
- **Best-effort collector:** a fetch/parse error logs at debug and returns `nil` — it never fails the inventory. (Same as `syncPolicies`, `client.go:263`.)
- **`days_to_expiry` is emitted only when the license has an expiration** — perpetual licenses (which omit `expiration`, so `days_to_expiry` is `0`) must NOT emit it, or a `< 30` alert false-fires.
- Fixtures are validated against the OneFS 9.14 OpenAPI spec by the schema-drift guard (`internal/powerscale/schema_guard_test.go` via `make schemas`); every fixture field must be documented in the spec.
- A Semgrep hook scans files on write and **blocks on findings**; inline `// nosemgrep` is not honored — fix by restructuring. Test HTTP handlers must write through the `writeBytes(io.Writer, …)` helper (already in `mockserver_test.go`), never directly to a `ResponseWriter`.
- Branch: `feat/license-metrics` (already created; the spec is committed there).

---

### Task 1: License model + `ParseLicenses`

**Files:**

- Modify: `internal/models/onefs.go` (add `License` type, `ParseLicenses`, `Inventory.Licenses` field)
- Test: `internal/models/onefs_test.go`

**Interfaces:**

- Consumes: `encoding/json`, `strings` (both already imported in `onefs.go`).
- Produces: `type License struct { Name, Status string; DaysToExpiry int; HasExpiration, Expired bool }`; `func ParseLicenses(b []byte) ([]License, error)`; `Inventory.Licenses []License`.

- [ ] **Step 1: Write the failing test**

Add to `internal/models/onefs_test.go`:

```go
func TestParseLicenses(t *testing.T) {
 data := []byte(`{"licenses":[
  {"name":"SyncIQ","status":"Licensed","expiration":"2027-01-01","days_to_expiry":214,"expired_alert":false},
  {"name":"SmartQuotas","status":"Expired","expiration":"2026-01-01","days_to_expiry":0,"expired_alert":true},
  {"name":"SnapshotIQ","status":"Licensed","days_to_expiry":0,"expired_alert":false},
  {"name":"CloudPools","status":"Evaluation","expiration":"2026-08-01","days_to_expiry":19,"expired_alert":false}
 ]}`)
 ls, err := ParseLicenses(data)
 if err != nil || len(ls) != 4 {
  t.Fatalf("parse: %d err=%v", len(ls), err)
 }
 if ls[0] != (License{Name: "SyncIQ", Status: "Licensed", DaysToExpiry: 214, HasExpiration: true, Expired: false}) {
  t.Fatalf("license[0]: %+v", ls[0])
 }
 if !ls[1].Expired {
  t.Fatalf("license[1] should be expired: %+v", ls[1])
 }
 if ls[2].HasExpiration {
  t.Fatalf("license[2] (perpetual, no expiration) should have HasExpiration=false: %+v", ls[2])
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestParseLicenses -v`
Expected: FAIL — `undefined: ParseLicenses` / `undefined: License`.

- [ ] **Step 3: Implement the model + parser**

In `internal/models/onefs.go`, add the field to the `Inventory` struct (after `Dedupe DedupeSummary`):

```go
 Licenses     []License
```

And add the type + parser (place near `ParseSyncPolicies`):

```go
// License is one OneFS licensed feature (license/licenses). HasExpiration is false for
// perpetual licenses (they omit the expiration field), so callers can skip emitting a
// meaningless days-to-expiry for them.
type License struct {
 Name          string
 Status        string
 DaysToExpiry  int
 HasExpiration bool
 Expired       bool
}

// ParseLicenses parses the license/licenses response into per-feature license state.
func ParseLicenses(b []byte) ([]License, error) {
 var raw struct {
  Licenses []struct {
   Name         string `json:"name"`
   Status       string `json:"status"`
   Expiration   string `json:"expiration"`
   DaysToExpiry int    `json:"days_to_expiry"`
   ExpiredAlert bool   `json:"expired_alert"`
  } `json:"licenses"`
 }
 if err := json.Unmarshal(b, &raw); err != nil {
  return nil, err
 }
 out := make([]License, 0, len(raw.Licenses))
 for _, l := range raw.Licenses {
  out = append(out, License{
   Name:          l.Name,
   Status:        l.Status,
   DaysToExpiry:  l.DaysToExpiry,
   HasExpiration: strings.TrimSpace(l.Expiration) != "",
   Expired:       l.ExpiredAlert,
  })
 }
 return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -run TestParseLicenses -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/onefs.go internal/models/onefs_test.go
git commit -m "feat(models): License type and ParseLicenses"
```

---

### Task 2: `licenseSamples` builder + label helpers

**Files:**

- Modify: `internal/powerscale/metrics.go` (add `licenseLabels`, `licenseInfoLabels`)
- Modify: `internal/powerscale/derivations.go` (add `licenseSamples`, wire into `BuildSamples`)
- Test: `internal/powerscale/derivations_test.go`

**Interfaces:**

- Consumes: `models.License` (Task 1); `baseLabels`, `b2f`, `Sample`, `Label` (existing).
- Produces: `licenseLabels(clusterName, clusterID, name string) []Label`; `licenseInfoLabels(clusterName, clusterID, name, status string) []Label`; `licenseSamples(clusterName, clusterID string, licenses []models.License) []Sample`; emits metric names `powerscale_license_days_to_expiry`, `powerscale_license_expired`, `powerscale_license_info`.

- [ ] **Step 1: Write the failing test**

Add to `internal/powerscale/derivations_test.go`:

```go
func TestBuildSamplesLicenses(t *testing.T) {
 inv := &models.Inventory{
  Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
  Licenses: []models.License{
   {Name: "SyncIQ", Status: "Licensed", DaysToExpiry: 214, HasExpiration: true, Expired: false},
   {Name: "SmartQuotas", Status: "Expired", DaysToExpiry: 0, HasExpiration: true, Expired: true},
   {Name: "SnapshotIQ", Status: "Licensed", DaysToExpiry: 0, HasExpiration: false, Expired: false},
  },
 }
 samples := BuildSamples("clu1", inv, nil)
 find := func(name, feature string) (Sample, bool) {
  for _, s := range samples {
   if s.Name != name {
    continue
   }
   for _, l := range s.Labels {
    if l.Name == "name" && l.Value == feature {
     return s, true
    }
   }
  }
  return Sample{}, false
 }
 if s, ok := find("powerscale_license_days_to_expiry", "SyncIQ"); !ok || s.Value != 214 {
  t.Fatalf("SyncIQ days_to_expiry wrong: %+v ok=%v", s, ok)
 }
 if _, ok := find("powerscale_license_days_to_expiry", "SnapshotIQ"); ok {
  t.Fatal("perpetual license (SnapshotIQ) must not emit days_to_expiry")
 }
 if s, ok := find("powerscale_license_expired", "SmartQuotas"); !ok || s.Value != 1 {
  t.Fatalf("SmartQuotas expired should be 1: %+v ok=%v", s, ok)
 }
 s, ok := find("powerscale_license_info", "SyncIQ")
 if !ok || s.Value != 1 {
  t.Fatalf("SyncIQ info missing: %+v ok=%v", s, ok)
 }
 hasStatus := false
 for _, l := range s.Labels {
  if l.Name == "status" && l.Value == "Licensed" {
   hasStatus = true
  }
 }
 if !hasStatus {
  t.Fatalf("info sample missing status label: %+v", s.Labels)
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesLicenses -v`
Expected: FAIL — `licenseSamples` not wired / metrics absent.

- [ ] **Step 3: Add label helpers**

In `internal/powerscale/metrics.go` (after `severityLabels`):

```go
// licenseLabels appends a licensed-feature name.
func licenseLabels(clusterName, clusterID, name string) []Label {
 return append(baseLabels(clusterName, clusterID), Label{Name: "name", Value: name})
}

// licenseInfoLabels appends a licensed-feature name and its OneFS status string.
func licenseInfoLabels(clusterName, clusterID, name, status string) []Label {
 return append(baseLabels(clusterName, clusterID),
  Label{Name: "name", Value: name},
  Label{Name: "status", Value: status},
 )
}
```

- [ ] **Step 4: Add `licenseSamples` and wire it in**

In `internal/powerscale/derivations.go`, add the builder (near `syncSamples`):

```go
// licenseSamples emits per-feature license status. days_to_expiry is emitted only for
// licenses that carry an expiration (perpetual licenses omit it, so a 0 would false-fire a
// "< 30 days" alert). expired and info are emitted for every license.
func licenseSamples(clusterName, clusterID string, licenses []models.License) []Sample {
 var out []Sample
 for _, l := range licenses {
  out = append(out,
   Sample{Name: "powerscale_license_expired", Labels: licenseLabels(clusterName, clusterID, l.Name), Value: b2f(l.Expired)},
   Sample{Name: "powerscale_license_info", Labels: licenseInfoLabels(clusterName, clusterID, l.Name, l.Status), Value: 1},
  )
  if l.HasExpiration {
   out = append(out, Sample{
    Name:   "powerscale_license_days_to_expiry",
    Labels: licenseLabels(clusterName, clusterID, l.Name),
    Value:  float64(l.DaysToExpiry),
   })
  }
 }
 return out
}
```

Wire it into `BuildSamples` (add after the `dedupeSamples(...)` append line, ~derivations.go:32):

```go
 samples = append(samples, licenseSamples(clusterName, clusterID, inv.Licenses)...)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestBuildSamplesLicenses -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/powerscale/metrics.go internal/powerscale/derivations.go internal/powerscale/derivations_test.go
git commit -m "feat(powerscale): licenseSamples builder for license metrics"
```

---

### Task 3: Best-effort fetch, fixture, schema guard, and e2e coverage

**Files:**

- Modify: `internal/powerscale/client.go` (add `licenses` helper + `Inventory` literal + debug log)
- Create: `internal/powerscale/testdata/licenses.json`
- Modify: `tools/extract-schemas/main.go` (targets map) — then regenerate via `make schemas`
- Modify: `internal/powerscale/mockserver_test.go` (serve the fixture)
- Modify: `internal/powerscale/e2e_test.go` (presence map)

**Interfaces:**

- Consumes: `models.ParseLicenses`, `Inventory.Licenses` (Task 1); the emitted metric names (Task 2); `c.getRaw`, `snippet`, `log` (existing).
- Produces: `func (c *ClusterClient) licenses(ctx context.Context) []models.License`; the `licenses.json` fixture; end-to-end emission of the three license metrics.

- [ ] **Step 1: Add the three license metrics to the e2e presence map (failing first)**

In `internal/powerscale/e2e_test.go`, add to the `want := map[string]bool{ … }` block:

```go
  "powerscale_license_days_to_expiry": false,
  "powerscale_license_expired":        false,
  "powerscale_license_info":           false,
```

- [ ] **Step 2: Run the e2e test to verify it fails**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: FAIL — `missing metric powerscale_license_*` (nothing fetches/serves licenses yet).

- [ ] **Step 3: Add the fixture**

Create `internal/powerscale/testdata/licenses.json`:

```json
{"licenses": [
  {"name": "SyncIQ", "status": "Licensed", "expiration": "2027-01-01", "days_to_expiry": 214, "expired_alert": false},
  {"name": "SmartQuotas", "status": "Expired", "expiration": "2026-01-01", "days_to_expiry": 0, "expired_alert": true},
  {"name": "SnapshotIQ", "status": "Licensed", "days_to_expiry": 0, "expired_alert": false},
  {"name": "CloudPools", "status": "Evaluation", "expiration": "2026-08-01", "days_to_expiry": 19, "expired_alert": false}
]}
```

- [ ] **Step 4: Serve the fixture from the mock server**

In `internal/powerscale/mockserver_test.go`, add a case to the path switch (before `default`), mirroring the existing cases:

```go
  case strings.HasSuffix(p, "/license/licenses"):
   writeBytes(w, fixture(t, "licenses.json"))
```

- [ ] **Step 5: Add the best-effort fetch + wire into `GetInventory`**

In `internal/powerscale/client.go`, add the helper (after `syncPolicies`, ~line 275):

```go
// licenses fetches OneFS license status best-effort (a missing ISI_PRIV_LICENSE privilege
// or an older release simply yields no license metrics).
func (c *ClusterClient) licenses(ctx context.Context) []models.License {
 var b []byte
 if err := c.getRaw(ctx, "platform/5/license/licenses", &b); err != nil {
  log.Debugf("cluster %q: licenses failed: %v", c.name, err)
  return nil
 }
 l, err := models.ParseLicenses(b)
 if err != nil {
  log.Debugf("cluster %q: parse licenses failed: %v; payload: %s", c.name, err, snippet(b))
  return nil
 }
 return l
}
```

Add `Licenses` to the `Inventory{}` literal in `GetInventory` (after `Dedupe: c.dedupeSummary(ctx),`, ~line 215):

```go
  Licenses:     c.licenses(ctx),
```

(Optional: extend the debug summary log at ~client.go:222-226 with `licenses=%d` / `len(inv.Licenses)`.)

- [ ] **Step 6: Run the e2e test to verify it passes**

Run: `go test ./internal/powerscale/ -run TestEndToEndCollectionThroughPrometheus -v`
Expected: PASS — the three `powerscale_license_*` metrics are present.

- [ ] **Step 7: Wire the endpoint into the schema-drift guard and regenerate**

In `tools/extract-schemas/main.go`, add to the `targets` map (after the sync/policies entry):

```go
 "/platform/5/license/licenses":              "licenses.json",
```

Then regenerate the extracted schemas so the guard validates the new fixture:

Run: `make schemas`
Expected: `internal/powerscale/testdata/onefs_schemas.json` is updated to include the license endpoint schema (git diff shows it).

- [ ] **Step 8: Run the full package (schema guard included)**

Run: `go test ./internal/powerscale/`
Expected: `ok` — including `schema_guard_test.go` asserting every `licenses.json` field (`name`, `status`, `expiration`, `days_to_expiry`, `expired_alert`) is documented in the 9.14 spec. If the guard fails on a field, that field is not in the v5 schema — remove it from the fixture and the parser rather than suppressing the guard.

- [ ] **Step 9: Commit**

```bash
git add internal/powerscale/client.go internal/powerscale/testdata/licenses.json internal/powerscale/testdata/onefs_schemas.json tools/extract-schemas/main.go internal/powerscale/mockserver_test.go internal/powerscale/e2e_test.go
git commit -m "feat(powerscale): best-effort license collector + schema guard + e2e coverage"
```

---

### Task 4: Documentation

**Files:**

- Modify: `docs/metrics.md` (new `### Licenses` section)
- Modify: `docs/getting-started/configuration.md` (add `ISI_PRIV_LICENSE`)
- Modify: `docs/getting-started/installation.md` (add `ISI_PRIV_LICENSE`)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Add the Licenses section to `docs/metrics.md`**

Add a new `### Licenses` subsection under the cluster-level metrics (e.g. after the SyncIQ/events/dedupe material):

```markdown
### Licenses

Per-feature OneFS license status, fetched best-effort from `license/licenses` (requires
`ISI_PRIV_LICENSE`; absent if the account lacks it). `days_to_expiry` is emitted **only for
licenses that carry an expiration** — perpetual licenses omit it.

| Metric | Labels | Description |
|---|---|---|
| `powerscale_license_days_to_expiry` | `name` | Days until the feature's license expires (omitted for perpetual licenses). |
| `powerscale_license_expired` | `name` | `1` when OneFS flags the feature's license as expired, else `0`. |
| `powerscale_license_info` | `name`, `status` | Constant `1`; the raw OneFS license `status` is carried as a label. |

Alert on a feature expiring within 30 days:

```promql
powerscale_license_days_to_expiry < 30
```

```

- [ ] **Step 2: Add `ISI_PRIV_LICENSE` to `configuration.md`**

In `docs/getting-started/configuration.md`, add this line to the privilege list (after the `ISI_PRIV_NFS` line):

```

# ISI_PRIV_LICENSE     (license status & expiry)

```

- [ ] **Step 3: Add `ISI_PRIV_LICENSE` to `installation.md`**

In `docs/getting-started/installation.md`, change the tail of the privilege sentence from:

```

`ISI_PRIV_SMB`, and `ISI_PRIV_NFS`. Create a dedicated monitoring user rather than reusing

```

to:

```

`ISI_PRIV_SMB`, `ISI_PRIV_NFS`, and `ISI_PRIV_LICENSE`. Create a dedicated monitoring user rather than reusing

```

- [ ] **Step 4: Verify the docs build**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds. (If `uvx` is unavailable, skip and note it — no Markdown change here would break the build.)

- [ ] **Step 5: Commit**

```bash
git add docs/metrics.md docs/getting-started/configuration.md docs/getting-started/installation.md
git commit -m "docs: document license metrics and ISI_PRIV_LICENSE privilege"
```

---

### Task 5: Final gate — full CI

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: gofmt clean, `go vet` clean, `golangci-lint` 0 issues, `go test -race` all packages pass (models + powerscale include the new license tests and the schema guard), `govulncheck` clean.

- [ ] **Step 2: (No commit)** — this task only gates the branch as green.

---

## Self-Review

**Spec coverage:**

- Spec "Source" (`platform/5/license/licenses`, best-effort) → Task 3 Step 5. ✅
- Spec "Metrics" (3 metrics; `days_to_expiry` only when `HasExpiration`; `expired` from `expired_alert`; `info` w/ status) → Task 2 (`licenseSamples`) + Task 1 (`Expired`/`HasExpiration` fields). ✅
- Spec "Data flow" (model+parse / client helper+Inventory / derivations) → Tasks 1, 2, 3. ✅
- Spec "Testing" (fixture w/ 4 rows incl. perpetual; schema guard; mock case; e2e + derivations asserts incl. perpetual-omits-days) → Task 1 (parse test), Task 2 (perpetual-omission assert), Task 3 (fixture, schema guard, mock, e2e). ✅
- Spec "Docs" (metrics.md section + ISI_PRIV_LICENSE) → Task 4. ✅
- Spec "Risks" (status values runtime, privilege/version best-effort) → handled by the best-effort design; no task needed.

**Placeholder scan:** every code/test/fixture step carries complete content; the only soft spot is the docs section's insertion point ("after the SyncIQ/events/dedupe material"), which is low-risk for a standalone subsection. ✅

**Type consistency:** `License{Name, Status, DaysToExpiry, HasExpiration, Expired}`, `ParseLicenses`, `Inventory.Licenses`, `licenses(ctx)`, `licenseSamples`, `licenseLabels`, `licenseInfoLabels`, and the three metric names are used identically across Tasks 1–4. `BuildSamples(clusterName, inv, st)` called with `st=nil` in Task 2 is safe (all `st`-based builders guard `st == nil`). ✅
