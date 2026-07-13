# OneFS 9.14.0 Spec Validation & Remediation — Design

**Date:** 2026-06-13
**Status:** Approved for planning
**Source of truth:** OneFS 9.14.0 OpenAPI spec (`11035-9.14.0.json`, OpenAPI 3.1, info.version 25)

## Problem

The exporter's `models.Parse*` functions were validated only against hand-authored
`testdata/` fixtures. Those fixtures were written to match the parsers, not the real
OneFS API — so the test suite validates the parser against itself. Cross-checking every
endpoint's parsed fields against the documented 9.14.0 response schemas revealed that
**five collection paths silently produce zero/empty data on a real cluster**, plus one
version gap and minor cleanup. Because every affected path is best-effort (a failure logs
at debug and yields a zero value), these defects are invisible in metrics — the exporter
reports healthy while emitting nothing for those series.

## Findings

Cross-checked: 14 endpoints, both path/version existence and every field read by a
`Parse*` function, against each endpoint's documented `200` response schema (with `$ref`
resolution).

| # | Endpoint (version we call) | Severity | Defect | Real-cluster effect |
| --- | --- | --- | --- | --- |
| 1 | `dedupe/dedupe-summary` v1 | Broken | Reads `logical_saving`/`logical_deduplicated`; schema has only block-based fields | Dedupe metrics always 0 |
| 2 | `statistics/summary/drive` v3 | Broken | Top key `drives`→ spec `drive`; items `lnn`/`bay`/`op_rate` → spec `drive_id`/`type`/`busy`/`xfers_*` | Drive metrics always empty |
| 3 | `statistics/summary/client` v3 | Broken | Reads `ops`; schema has `operation_rate`/`num_operations` | Client ops always 0 |
| 4 | `statistics/summary/protocol` **v2** | Wrong version | 9.14 documents only v3; fields match v3 | 404 → silently empty |
| 5 | `sync/policies` **v11** | Wrong version | 9.14 has v1/3/7/14/18, no v11; fields match all | 404 → silently empty |
| 6 | `quota/quotas` **v1** | Version gap | Reads `usage.fslogical`/`fsphysical`; v1 schema has only `logical`/`physical`/`inodes` (extended fields appear in v7+/v8+) | Quota usage may be 0 |
| 7 | `cluster/nodes` smartfail | Cosmetic | `state.smartfail.state` string absent in 9.14 (booleans only); `smartfailed` bool already covers it | None — dead branch |
| 8 | `cache.*` stat keys | Unverifiable | Stat-key names are runtime values (`/statistics/keys` catalog), not in OpenAPI | Live validation still pending |

Validates cleanly at the version called: `cluster/config` v3, `cluster/nodes` v3 (identity

+ health), `snapshot/snapshots-summary` v1, `event/eventgroup-occurrences` v3,
`statistics/current` v1, and the NFS/SMB/snapshot `total` counts.

Fixture confirmation (proves the circular-validation trap): `dedupe_summary.json` uses
`logical_saving`; `stat_drive.json` uses `drives`/`lnn`/`bay`/`op_rate`; `stat_client.json`
uses `ops`; `quotas.json` uses `fslogical`/`fsphysical` — none match the v1/v3 schemas the
exporter actually queries.

## Remediation

### Part A — Fix the three broken parsers (`internal/models/onefs.go`)

+ **`ParseDedupeSummary`**: read `summary.{saved_logical_blocks, logical_blocks, block_size}`
  (all `number`). Derive `LogicalSavedBytes = saved_logical_blocks × block_size` and
  `DeduplicatedBytes = logical_blocks × block_size`. Keep the `DedupeSummary` field names
  and the emitted metric names unchanged (semantics preserved: bytes saved / bytes
  deduplicated). Keep pointer/nil-safe handling.
+ **`ParseDriveSummary`**: top-level key `drive` (singular). Per item: split `drive_id`
  (`"LNN:bay"`) on `:` into `Node` (int LNN) and `Bay` (string); keep `type`→`Type` and
  `busy`→`BusyPercent`; set `OpsPerSec = xfers_in + xfers_out` (write + read op rates).
  Malformed `drive_id` → skip row (best-effort), logged at debug.
+ **`ParseClientSummary`**: `OpsPerSec` reads `operation_rate` (not `ops`). `in`/`out`/
  `node`/`protocol`/`class` already correct.

### Part B — Correct three endpoint versions (`internal/powerscale/client.go`)

+ protocol summary: `platform/2/statistics/summary/protocol` → **`platform/3/...`**.
+ sync policies: `platform/11/sync/policies` → **`platform/7/sync/policies`** (mid version;
  present on 9.14 and recent 9.x; carries name/enabled/last_job_state).
+ quota: `platform/1/quota/quotas` → **`platform/8/quota/quotas`** (lowest version whose
  schema documents both `fslogical` and `fsphysical`; v7 has `fslogical` but not
  `fsphysical`).

### Part C — Regenerate fixtures + update tests (`internal/powerscale/testdata/`, `*_test.go`)

Rewrite to real spec field names so tests exercise corrected parsers against truthful
payloads:
+ `dedupe_summary.json` → block-based fields.
+ `stat_drive.json` → `drive` array with `drive_id`, `type`, `busy`, `xfers_in`, `xfers_out`.
+ `stat_client.json` → `operation_rate` instead of `ops`.
+ `quotas.json` → confirm `fslogical`/`fsphysical` (already used; now matched by the v8 path).

Update expected values in `models/onefs_test.go` and `powerscale/*_test.go` (mockserver
serves these fixtures) to the recomputed metrics (e.g. dedupe bytes = blocks × block_size,
drive ops = xfers_in + xfers_out).

### Part D — Cosmetic / documentation

+ Remove the dead `smartfail.state` string branch in `ParseNodes`; keep the `smartfailed`
  boolean. (Defensive-for-old-schema value is negligible; the field is gone in 9.14.)
+ Add a code comment at the `cache.*` rows in `statisticsKeys.json` / nearby that these
  keys are validated only at runtime via `/platform/1/statistics/keys`, not from OpenAPI.

### Part E — Spec-drift guard (trimmed spec + Go test)

Prevent fixtures from silently diverging from the schema again:

1. **Vendor a trimmed schema set** at `internal/powerscale/testdata/onefs_schemas.json`:
   a small JSON (tens of KB) containing, per endpoint we use, the resolved set of
   documented field paths for its `200` response (extracted from the full 9.14.0 spec with
   `$ref`s flattened). Generated by a committed helper script
   (`tools/extract-schemas/` or a `//go:generate`-able Go program) so it is reproducible
   when a new OneFS spec drops.
2. **Add a Go test** (`internal/powerscale/schema_guard_test.go`) that, for each fixture,
   asserts every JSON field present in the fixture is a documented field path in
   `onefs_schemas.json` for that endpoint. Fails CI if a fixture introduces an
   undocumented field (the exact trap that hid these defects). Runs inside `make ci`,
   self-contained, no network.
3. The guard checks **fixtures ⊆ schema** (no invented fields). It deliberately does not
   assert schema ⊆ fixtures (the API documents far more than we consume).

## Out of scope

+ Live-cluster validation of `cache.*` stat-key names (tracked separately;
  requires cluster access — see `provisional-onefs-keys` memory).
+ Using the SDK-negotiated per-cluster APIVersion in request paths (larger change;
  resources have non-uniform version availability, e.g. dedupe is v1-only). Versions
  stay pinned per endpoint.
+ New metrics from now-available schema fields (e.g. drive `access_latency`,
  `used_bytes_percent`; client `time_avg`). Possible follow-up, not this remediation.

## Verification

+ `make ci` green (fmt, vet, golangci-lint, `go test -race`, govulncheck).
+ New schema-guard test fails when a fixture field is undocumented (add a deliberately-bad
  fixture field in a throwaway check to prove it bites, then revert).
+ `--once --debug` against the mock server shows non-zero dedupe/drive/client samples.
+ Semgrep write-hook passes (fix by restructuring, never `// nosemgrep`).
