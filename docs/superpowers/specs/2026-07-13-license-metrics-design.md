# Design: OneFS license status & expiry metrics (#34)

Date: 2026-07-13
Status: approved for planning
Issue: #34 (first of three follow-up collectors from the v0.13.0 review; #32 workload and #33 storagepool are separate cycles)
Scope: `internal/models/onefs.go`, `internal/powerscale/client.go`, `internal/powerscale/derivations.go`, `tools/extract-schemas/main.go`, `internal/powerscale/testdata/`, collector/e2e tests, `docs/metrics.md`.

## Goal

Expose per-feature OneFS license status and expiry so operators can alert **before** a licensed feature (SyncIQ, SmartQuotas, …) lapses. This is the clearest alerting payoff of the three follow-ups and the simplest schema.

## Why this is low-risk

Unlike the cache stat keys (runtime values absent from the OpenAPI spec), the license fields are **structural fields documented in the OneFS OpenAPI schema**, so the schema-drift guard fully validates them. OneFS also pre-computes the expiry math (`days_to_expiry`, `expired_alert`), so the exporter does no date arithmetic. The only runtime (non-schema) values are the exact `status` strings, which need a light live check but do not affect structure. There is no cluster-side prerequisite (contrast #32 workload).

## Source

`GET /platform/5/license/licenses` — v5 is present in both the 9.13.0 and 9.14.0 specs and already carries every field below (v17/v24 also exist; v5 chosen for widest compatibility). Fetched **best-effort**, exactly like `syncPolicies`/`dedupeSummary` (a missing `ISI_PRIV_LICENSE` privilege or an older release simply yields no license metrics; the exporter and all other collectors keep running).

Response shape (`{ "licenses": [ … ] }`), per the 9.14 schema — fields consumed:

| field | type | use |
|---|---|---|
| `name` | string | feature name → `name` label |
| `status` | string | current license status → `status` label |
| `expiration` | string (`YYYY-MM-DD`) | **presence** flag only — omitted when the license is perpetual |
| `days_to_expiry` | integer | value of `powerscale_license_days_to_expiry` |
| `expired_alert` | boolean | value of `powerscale_license_expired` |

`id`, `days_since_expiry`, `expiring_alert`, `tiers`, and the top-level fields are not consumed.

## Metrics

All best-effort; an empty/failed fetch emits nothing. Canonical leading labels `cluster`, `cluster_id`.

| Metric | Labels | Value | Emitted for |
|---|---|---|---|
| `powerscale_license_days_to_expiry` | `cluster, cluster_id, name` | `days_to_expiry` | **only licenses that have an `expiration`** |
| `powerscale_license_expired` | `cluster, cluster_id, name` | `1` if `expired_alert` else `0` | every license |
| `powerscale_license_info` | `cluster, cluster_id, name, status` | constant `1` | every license |

**The perpetual-license rule is the one correctness detail:** a perpetual license omits `expiration`, and its `days_to_expiry` is `0`. Emitting `days_to_expiry=0` would false-fire a `days_to_expiry < 30` alert, so `powerscale_license_days_to_expiry` is emitted **only when `HasExpiration` is true**. `powerscale_license_expired` and `powerscale_license_info` are emitted for every license (a perpetual license is simply `expired=0`, `status="Licensed"`).

`powerscale_license_info` follows the Prometheus info-metric pattern (constant `1`, state carried in the `status` label), so no brittle "is it active" interpretation is baked in — users filter on `status`. License count is small (~10–20 features), so info-label churn on status change is not a cardinality concern.

## Data flow (established best-effort typed-collector pattern)

1. **`internal/models/onefs.go`**
   - `type License struct { Name string; Status string; DaysToExpiry int; HasExpiration bool; Expired bool }`
   - `func ParseLicenses(b []byte) ([]License, error)` — unmarshal `{ "licenses": [...] }`; set `HasExpiration = strings.TrimSpace(raw.Expiration) != ""`, `Expired = raw.ExpiredAlert`.
   - Add `Licenses []License` to the `Inventory` struct (onefs.go:145).
2. **`internal/powerscale/client.go`**
   - New helper `func (c *ClusterClient) licenses(ctx context.Context) []models.License`, modeled exactly on `syncPolicies` (client.go:263): `getRaw(ctx, "platform/5/license/licenses", &b)`; on error `log.Debugf(...); return nil`; else `ParseLicenses`, on parse error `log.Debugf(...); return nil`.
   - Add `Licenses: c.licenses(ctx),` to the `Inventory{}` literal in `GetInventory` (client.go:207-216).
   - Optional: extend the debug summary log (client.go:~226) with `len(inv.Licenses)`.
3. **`internal/powerscale/derivations.go`**
   - `func licenseSamples(clusterName, clusterID string, licenses []models.License) []Sample` — for each license: always append `powerscale_license_expired` (`b2f(l.Expired)`) and `powerscale_license_info` (value `1`, labels incl. `status`); append `powerscale_license_days_to_expiry` only when `l.HasExpiration`.
   - Wire into `BuildSamples` (derivations.go:22-35) via `inv.Licenses`.
   - Add label helpers as needed (reuse `baseLabels`; add a `licenseLabels`/`licenseInfoLabels` following the existing `*Labels` helpers in metrics.go).

## Testing

- **`internal/powerscale/testdata/licenses.json`** — four rows exercising every branch:
  - active with expiry (`expiration` set, `days_to_expiry` > 0, `expired_alert` false) → days series emitted, `expired=0`;
  - expired (`expired_alert` true) → `expired=1`;
  - **perpetual** (no `expiration`) → **no `days_to_expiry` series**, `expired=0`;
  - evaluation (`status:"Evaluation"`, has `expiration`).
- **Schema guard:** add `"/platform/5/license/licenses": "licenses.json"` to the `targets` map in `tools/extract-schemas/main.go`, then `make schemas` so `schema_guard_test.go` asserts every fixture field is documented in the 9.14 spec.
- **Mock server:** add a `strings.HasSuffix(p, "/license/licenses")` case to `mockserver_test.go` serving `licenses.json` via `writeBytes`.
- **Assertions:**
  - `e2e_test.go`: add `powerscale_license_days_to_expiry`, `powerscale_license_expired`, `powerscale_license_info` to the presence map.
  - `derivations_test.go`: assert the perpetual license emits **no** `powerscale_license_days_to_expiry` while still emitting `expired`/`info`, and that an expired row yields `expired=1`.
  - Follow the existing dual-path style (Prometheus registry gather + OTLP `ManualReader`) used by the collector tests.

## Docs

- New `### Licenses` section in `docs/metrics.md` documenting the three metrics, the perpetual-license omission of `days_to_expiry`, and the alert example:
  ```promql
  # a licensed feature expiring within 30 days
  powerscale_license_days_to_expiry < 30
  ```
- Add `ISI_PRIV_LICENSE` to the documented read-only privilege set (configuration.md / installation.md). Note it is best-effort: without the privilege, license metrics are simply absent.

## Non-goals

- The `tiers` array, `days_since_expiry`, `expiring_alert`, `swid`/`valid_signature`/`activation_incomplete_alert` top-level fields. YAGNI.
- Absolute `_expiration_timestamp_seconds` — rejected in favor of the cluster's own `days_to_expiry` (API-native, no date parsing).
- #32 workload and #33 storagepool — separate cycles.

## Risks

- **`status` string values** are runtime, not enumerated in the schema. Structure is spec-validated; the exact set of `status` values (e.g. `Licensed` / `Evaluation` / `Expired` / `Unlicensed`) should be spot-checked against a live cluster, but a new value only appears as a label — it cannot break collection.
- **`ISI_PRIV_LICENSE` name** unconfirmed against Dell RBAC docs; if the privilege differs, the docs line is corrected and the best-effort design means collection is unaffected either way.
- **Endpoint version:** if a target OneFS predates v5 `/license/licenses`, the fetch fails best-effort (no metrics), matching every other optional collector.
