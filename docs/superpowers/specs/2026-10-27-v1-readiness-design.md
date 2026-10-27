# v1.0 readiness — design

Date: 2026-10-27. Status: draft, awaiting approval.

## Intent

pscale_exporter is a public, free, Apache-2.0 project that other people install against
production Dell PowerScale clusters. Version 1.0.0 should tell a stranger: *the
interface you build dashboards and alerts on will not break without a major bump, and
the repo is safe and easy to adopt.* The code, CI and supply chain are already at that
level (SBOMs, attestations, SHA-pinned actions, govulncheck, Semgrep, non-root image).
What is missing is the **promise** and the **public-project packaging**, plus evidence
for the few metrics never seen on a real cluster.

## Goals

1. A written **stability policy** (an ADR) that fixes what v1 promises and what it does not.
2. **Public-project packaging**: security note in the README, `SECURITY.md`,
   `CONTRIBUTING.md`, and protection of `main` (last, a bonus).
3. **Evidence for unvalidated metrics**, or an explicit "provisional" label when no
   evidence exists.
4. **Tests** for the poorly covered entry-point packages: `main`, `internal/config`,
   `internal/telemetry`.
5. A clean changelog so 1.0.0 starts from a true record.

## Non-goals

- No new metrics and no behaviour change in collection. Renames happen only in the
  naming audit below, only the ones the maintainer approves one by one.
- No authentication or TLS on `/metrics`. It stays an open Prometheus endpoint; the
  README states how to protect it (reverse proxy or trusted network).
- No restructuring of `planning/` or other historical files.
- No raising of coverage on `internal/powerscale` or `internal/models` (already ~80% / ~72%).

## Anti-goals

- A v1 that promises things we do not control or have never observed.
- Governance that blocks the only maintainer from merging their own work.
- Rules invented for this repo that contradict the sibling exporters in the family.

## The stability policy (ADR-0007)

Next free ADR number is 0007. (CLAUDE.md says "7 ADRs"; the directory holds 0001–0006.
Correct the count in the same PR.)

### Promised in v1 (breaking one needs a major bump)

- Metric **names**, **label names** and **units** of every stable `powerscale_*` metric.
  Adding a label to an existing series counts as breaking: it changes series identity for
  users' dashboards and alerts.
- CLI flags, `config.yaml` keys, the `${VAR}` / `${VAR:-default}` semantics, the default
  port 9444, the JSON body of `/health` (the top-level keys it returns today; ADR-0005 and
  `docs/troubleshooting.md` describe it, and the ADR cites them rather than adding a new
  schema, *my call*), and the Helm chart `values.yaml` keys.
- Adding a new metric, a new label *value*, or a new optional config key is not breaking.

### Not promised: bounded by Dell

The exporter reads what OneFS returns and depends on Dell's `gopowerscale` SDK. So:

- The **presence and value** of a metric depend on the cluster's OneFS version and API
  version (for example, quota API v1 has no `fslogical`/`fsphysical`; 9.15 adds new
  fields). A metric can be absent or zero on an older cluster without that being a bug.
- Dell can change an endpoint, a stat key or its units in a later OneFS release. We fix
  forward and record it in the CHANGELOG; this is not a v1 break by us.
- The SDK is tracked like any dependency; its upgrades follow the same rule.
- **Dell dashboard compatibility** is best effort. We keep the `powerscale_` prefix of
  `dell/csm-metrics-powerscale` (ADR-0004) but do not follow Dell if they rename theirs.

### Provisional tier (outside the v1 guarantee)

Listed in the ADR and in `docs/metrics.md`, each with a one-line reason:

- `powerscale_quota_reduction_ratio` (OneFS 9.15, from the spec, never seen live).
- `powerscale_protocol_latency_stage_microseconds` (OneFS 9.15, same).
- `powerscale_quota_application_usage_bytes` (`usage.applogical`, fixtures only).
- The node hardware block, five metrics, provisional as a unit:
  `powerscale_node_power_supplies_total`, `powerscale_node_power_supply_failures`,
  `powerscale_node_temperature_celsius`, `powerscale_node_fan_speed_rpm`,
  `powerscale_node_hardware_info`. They are already grouped under a "Hardware (provisional)"
  heading in `docs/metrics.md`.

Stability is **per metric name**. `docs/metrics.md` marks each provisional metric with a
"provisional" note in its row (the heading alone is not enough once rows are quoted
elsewhere). No automated check enforces it in v1. *(My call.)*

A metric moves to stable after a live capture confirms its presence, unit and null
behaviour. That move is a minor release, not a major one.

## Packaging

- **README:** a short security section. `/metrics` is open and binds `0.0.0.0` by default,
  exposes cluster, quota path and node names, and should sit behind a reverse proxy or a
  trusted network. Mention `insecureSkipVerify` is off by default.
- **`SECURITY.md`:** supported versions (latest minor), reporting through GitHub private
  vulnerability reporting, what to expect. Enable the feature on the repo first (with the
  maintainer's OK), then merge `SECURITY.md`, so the link works the day it appears. No personal e-mail
  in the file. Model on the `kemp_exporter` one for family consistency; check the other
  siblings and the `exporter-standards` skill before writing, and adapt to this repo.
- **`CONTRIBUTING.md`:** how to build and test (`make ci`), the merge-commit rule, the
  metric-naming and stat-key conventions, how to report a bug with `--once --debug --trace`
  captures (with a warning about sensitive names in dumps). Same family model.
- **Protection of `main` (last, bonus):** required status check `ci / ci` only, no required
  reviews, since the sole maintainer cannot approve their own PR. Known caveat: CI does not
  re-run on a retargeted stacked PR, so the policy suits single PRs. The exact settings
  are shown to the maintainer before they are applied.
- **Changelog:** the entry now under Unreleased (`powerscale_quota_application_usage_bytes`,
  #54) *is* v0.21.0, released 2026-10-27. Move it under a dated `[0.21.0]` heading, and add
  the v1.0.0 entry when it ships.

## Evidence for unvalidated metrics

The maintainer can get a live capture through a colleague, once. A ready request exists
(French, one run: `--once --debug --trace --dump-dir`, version, privileges, QuotaScan state).

- When the capture arrives, compare it with the fixtures and `docs/metrics.md`. Fix the
  parsers or fixtures if they differ, then promote each confirmed metric from provisional
  to stable, and drop the "not yet validated" wording in the docs.
- If it never arrives, v1 ships with the provisional tier as written. The schedule does
  not wait on it. Optionally the reporter of issue #54 (OneFS 9.7) can be asked to run
  `--once --debug` for `applogical`; that is a public comment and needs the maintainer's OK.

## Tests

Add focused tests, not a coverage target for its own sake:

- `main`: server start and shutdown, `/metrics`, `/health`, `/livez`, `/readyz`, `--once`
  behaviour, config reload.
- `internal/config`: SIGHUP handler and file watcher.
- `internal/telemetry`: manager initialisation and shutdown with OTLP on and off.

Production code is not changed to add behaviour. If `main.go` (whose logic sits in one
`RunE` closure inside `main()`) cannot be tested as is, a small extraction into a function
with no behaviour change is allowed, in its own commit, and flagged in the PR. A real bug
found by a test is also fixed in its own commit. *(My call.)*

## Naming audit (decided: do it before 1.0.0)

The maintainer chose to audit metric and label names and units before freezing them,
because after 1.0.0 any rename is a major bump. This runs first. The audit is a report;
no rename is made until the maintainer approves it, item by item.

Scope: every `powerscale_*` name and label in `docs/metrics.md`, `internal/powerscale`,
`statisticsKeys.json`, the Grafana dashboards, the Helm chart and the docs. Check against
the Prometheus naming conventions (`promtool check metrics` on a real scrape) and the
rules in ADR-0004.

For each approved rename the same PR must: update code, fixtures, dashboards (keeping every
board `uid` stable), docs and the Helm chart; keep a CHANGELOG **BREAKING** entry with the
old and new name; and state the migration in the ADR. Whether to emit the old name for one
release as an alias is decided per rename when it is approved (default: no alias).

Findings so far (read from the code and `docs/metrics.md` only; `promtool` and the dashboards
are not yet checked):

1. **Gauges named `_total`.** Prometheus reserves `_total` for counters. Five count gauges
   carry it: `powerscale_nfs_exports_total`, `powerscale_smb_shares_total`,
   `powerscale_snapshots_total`, `powerscale_node_drives_total`,
   `powerscale_node_power_supplies_total`. A linter flags them. Candidate renames drop
   the suffix (`powerscale_nfs_exports`, ...). Recommended.
2. **Singular and plural.** `powerscale_snapshot_used_bytes` next to `powerscale_snapshots_total`.
   Resolved by item 1 if `snapshots_total` becomes `powerscale_snapshots`. Cosmetic.
3. **Quota labels** are `quota_id`, `quota_path`, `quota_type`, while other families use
   plain `type`, `name`, `pool`. The prefix keeps them distinct when quota series are
   joined with others. Recommended: keep.
4. **Overloaded `type`** (storage pool type, drive type) and `node` (always the node LNN).
   Understandable in context. Recommended: keep.
5. `free` and `available` capacity both exist at cluster level. They are different
   OneFS quantities, so not a duplication. To be confirmed against the docs.

## Delivery

Four PRs, in this order. (0) Naming audit report, then the approved renames, if any.
(a) Packaging + stability ADR + changelog fix. (b) Tests. (c) Validation results and
provisional promotions, when the capture exists; it is **not awaited**. 1.0.0 is tagged
once (0), (a) and (b) are merged, with the provisional tier as written; (c) ships later
as a minor release. Each PR must pass `make ci`.

## Decisions taken by the maintainer

1. Audit names, labels and units before freezing, and rename what is inconsistent.
2. Tag 1.0.0 without waiting for the live capture.

## Left to the builder

Exact wording of the documents, file layout of the new tests, which sibling exporter's
`SECURITY.md` / `CONTRIBUTING.md` is the closest model, and the order of work inside a PR.
