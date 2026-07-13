# Design: fix the OneFS cache stat-key prefix (`node.ifs.` restoration)

Date: 2026-07-13
Status: approved for planning
Scope: `internal/powerscale/statisticsKeys.json`, `internal/powerscale/testdata/stat_current.json`, collector tests, `docs/metrics.md`, project memory.

## Problem

A live OneFS cluster (OneFS on `tbs-isilona-1`) rejects the exporter's `statistics/current`
request:

```
GET platform/1/statistics/current: Invalid key: 'cache.l1.data.read.hit'
cluster "pscale-cluster1": statistics fetch failed: ... Invalid key: 'cache.l1.data.read.hit'
```

The six cache keys in `statisticsKeys.json` are written as `cache.lN.data.read.{hit,miss}`,
but the real OneFS keys (per `isi statistics list keys list`) are node-scoped under the
`node.ifs.` namespace:

```
node.ifs.cache.l1.data.read.hit / .miss
node.ifs.cache.l2.data.read.hit / .miss
node.ifs.cache.l3.data.read.hit / .miss
```

The `node.ifs.` prefix was dropped when these keys were first added (PR #2, derived from the
OneFS API reference PDF + SDK types, never validated live).

### This is a cluster-wide statistics outage, not just missing cache metrics

OneFS `statistics/current` is **all-or-nothing**: it rejects the *entire* batch on the first
invalid key. In `client.go:325-327`, `GetStatistics` returns that error and aborts before the
protocol/drive/client summaries even run. So while these six keys are wrong, the cluster loses
**every** `statistics/current` metric — CPU (`cluster.cpu.*`, `node.cpu.*`), capacity
(`ifs.bytes.*`, `node.ifs.bytes.used`), node memory, and disk — not only cache.

This disproves the assumption recorded in the `provisional-onefs-keys` memory note, which said a
wrong key "degrades to an empty series, not an error." That is true for a *response* row with
`error != null` (the parser skips it), but **not** for a batched request: one bad key 400s the
whole call.

## Investigation / cross-check (why the fix is what it is)

Validated against every available source before designing:

| Source | Cache keys? | Finding |
|---|---|---|
| Live cluster (`isi statistics list keys`) | yes | Authoritative. Keys are `node.ifs.cache.lN.data.read.{start,hit,miss,wait}`. |
| swagger 9.14 spec (`docs/swagger/11035-9.14.0.json`) | no | Stat-key *names* are runtime data, not schema. Spec has zero cache keys. It **does** document `GET /platform/1/statistics/keys/{key}` → `v1StatisticsKey{units, type, scope, real_name, aggregation_type}` — the machine-readable way to resolve units. |
| gopowerscale v1.22.0 (SDK) | no | Pure passthrough. `GetFloatStatistics(ctx, keys)` forwards key strings verbatim; no constants, no validation. It cannot catch a bad key — OneFS does. |
| csm-metrics-powerscale (Dell, our `powerscale_` dashboard reference) | no | Queries only 7 **cluster-scoped** keys (`ifs.bytes.total/avail`, `cluster.cpu.sys.avg`, `cluster.disk.{xfers,bytes}.{in,out}.rate`). No node-scoped keys, no cache. |
| csi-powerscale (Dell CSI driver) | no | Uses only `ifs.bytes.avail`. Not a metrics collector. |

Two conclusions:

1. **No external precedent for cache metrics.** Dell collects none, so `powerscale_node_cache_*`
   is original to this exporter. The live cluster's `statistics/keys` is the *sole* authority for
   both key strings and units — nothing external can confirm the `_bytes_per_second` suffix.
2. **The bug is structurally isolated to these six keys.** Every other key in `statisticsKeys.json`
   (and every Dell key) is cluster-scoped `ifs.*` / `cluster.*`, where no `node.` prefix applies and
   our strings are correct. The cache keys are the only node-scoped `ifs.*` keys we have, and
   node-scoped `ifs.*` stats live under `node.ifs.`. That's exactly the prefix that was dropped.

## Goals

- Restore `statistics/current` for affected clusters by correcting the six cache key strings.
- Close the test gap that let an invalid key ship (no fixture exercised the cache keys).
- Correct the misleading docs/memory claims about failure behavior.
- Provide a concrete, trace-based procedure to confirm units/type when a cluster is reachable.

## Non-goals

- Expanding cache coverage (meta cache, `.start`/`.wait`, prefetch, proper hit-ratio). Deferred.
- Splitting the batch `statistics/current` request into per-key requests (22 calls vs 1). Rejected.
- Changing the unrelated `cluster.disk.xfers.rate` vs Dell's in/out split. Out of scope.

## Design

### 1. Key-string fix (definite)

In `internal/powerscale/statisticsKeys.json`, prefix the six cache keys with `node.ifs.`:

| before | after |
|---|---|
| `cache.l1.data.read.hit` | `node.ifs.cache.l1.data.read.hit` |
| `cache.l1.data.read.miss` | `node.ifs.cache.l1.data.read.miss` |
| `cache.l2.data.read.hit` | `node.ifs.cache.l2.data.read.hit` |
| `cache.l2.data.read.miss` | `node.ifs.cache.l2.data.read.miss` |
| `cache.l3.data.read.hit` | `node.ifs.cache.l3.data.read.hit` |
| `cache.l3.data.read.miss` | `node.ifs.cache.l3.data.read.miss` |

`scope` stays `node`; `metric` names are **unchanged in this pass** (see §3). No code change —
`QueryKeys()` reads the JSON and `statSamples()` matches by key string.

### 2. Test coverage (close the regression gap)

The mock fixture never contained cache rows, so nothing tested these keys — which is why an
invalid key shipped green. Add coverage:

- **`internal/powerscale/testdata/stat_current.json`** — add six node-scoped rows using the same
  shape as existing rows, with a `devid` that maps to a node in `nodes.json` (e.g. `devid: 1` →
  lnn 1):
  ```json
  {"devid": 1, "error": null, "key": "node.ifs.cache.l1.data.read.hit",  "time": 1700000000, "value": 1000},
  {"devid": 1, "error": null, "key": "node.ifs.cache.l1.data.read.miss", "time": 1700000000, "value": 100},
  ... l2, l3 ...
  ```
- **Assertions** — mirror the `node_memory_used` pattern:
  - `internal/powerscale/e2e_test.go`: add the six `powerscale_node_cache_*` metric names to the
    presence map.
  - `internal/powerscale/derivations_test.go`: add a value+label assertion for at least one cache
    series (value present, node label = "1"), following `derivations_test.go:82`.
- The row with the **correct** key string is the durable guard: if a future edit drops the
  `node.ifs.` prefix again, `statKeyByKey` no longer matches and the assertion fails.

Note on the schema-drift guard (`schema_guard_test.go`): it checks that fixture *fields* (`devid`,
`error`, `key`, `time`, `value`) are documented in the spec — they are. The key *string* is a data
value, not a schema field, so adding cache rows does not trip the guard.

### 3. Units / metric naming — trace-gated, no guessing

The metric names currently claim `_bytes_per_second`. We cannot confirm rate-vs-counter without
live data, and inventing a name from convention would contradict the whole investigation. So:

- **This pass keeps the existing `_bytes_per_second` names** — the smallest correct change. Because
  the invalid key meant these series *never emitted*, no dashboard depends on the current names;
  any later rename has **zero** backward-compat cost.
- **Signal to check:** OneFS rate keys conventionally end in `.rate` (cf. `cluster.disk.xfers.rate`,
  `cluster.net.ext.bytes.in.rate`). The cache keys end in `.hit`/`.miss` with no `.rate`, which
  *suggests* cumulative **byte counters** — but this is a hypothesis, not a decision.
- **Validation procedure (run when a cluster is reachable):**
  1. `./bin/pscale_exporter --config config.yaml --trace` (looping) so the raw
     `platform/1/statistics/current` response body is logged each interval (`traceResponse`,
     `client.go:79`, covers this endpoint via `getRawParams`).
  2. Read the six cache rows' `value` across ≥2 consecutive intervals:
     - **monotonically increasing** cumulative bytes → **counter** → rename metrics to
       `powerscale_node_cache_lN_read_{hit,miss}_bytes` and change the PromQL hint to `rate(...)`;
       hit ratio = `sum(rate(hit)) / sum(rate(hit) + rate(miss))`.
     - **stationary** bytes/sec figure → **rate** → keep `_bytes_per_second`; aggregate with
       `sum`/`avg` (never `rate()`), consistent with the exporter's per-second-gauge convention.
  3. (Optional, exact) `GET /platform/1/statistics/keys/node.ifs.cache.l1.data.read.hit` returns
     `units` and `type` directly if API access is available.
- The rename, if needed, is a trivial follow-up (JSON `metric` field + test names + one doc line).

### 4. Docs & memory corrections

- **`docs/metrics.md`** (cache section, ~L55-66): replace the misleading caveat. Remove the
  "keys vary by release … emit nothing if your cluster uses different keys" wording (it implies a
  silent per-key skip). State the accurate behavior: keys are node-scoped `node.ifs.cache.*`,
  confirmed against OneFS via `isi statistics list keys` / `statistics/keys`; and a single invalid
  key fails the **entire** `statistics/current` batch (all-or-nothing). Keep a short note that the
  rate-vs-counter unit semantics are pending live `--trace` validation (§3); drop "(provisional)"
  only once that is done.
- **Project memory `provisional-onefs-keys.md`**: correct the "unknown keys are silently
  skipped / a wrong key degrades to an empty series, not an error" claim — for a batched `current`
  request, one invalid key 400s the whole call and takes down all statistics. Update the "STILL
  UNVALIDATED" section: cache key *strings* are now confirmed against the live cluster (2026-07-13);
  only the unit semantics (rate vs counter) remain pending `--trace`.

### 5. Optional hardening (flagged, deferred)

Because the batch is all-or-nothing, any future unvalidated key can silently take down *all*
statistics. Cheap mitigation: a prominent comment in `statisticsKeys.json` / `statkeys.go` warning
that every key must be validated against a live cluster before merge. Splitting into per-key
requests would isolate failures but turns 1 request into ~22 — not worth it. Comment only, if at all.

## Verification plan

- `make test` (or `go test ./internal/powerscale/ -run TestCollect`) — new cache assertions pass;
  schema-drift guard still green.
- `make ci` — fmt, vet, lint, `-race`, govulncheck.
- Live (when a cluster is reachable): `--once --debug` shows the six `powerscale_node_cache_*`
  samples and no "Invalid key" error; `--trace` confirms rate-vs-counter per §3.

## Risks

- **Other hidden invalid keys.** OneFS reports only the *first* invalid key, so fixing the cache
  keys could surface another. Mitigation: the remaining keys are canonical (`ifs.bytes.*`,
  `cluster.cpu.*`, Dell-confirmed) and low-risk; `--once --debug` on a live cluster is the final check.
- **Units unresolved until trace.** Accepted: names are kept as-is and the correction is a
  zero-compat-cost follow-up gated on trace evidence.
