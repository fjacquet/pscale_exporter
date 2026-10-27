# v1.0 stability policy

## Status

Accepted (2026-10-27)

## Context

pscale_exporter is a public, free, Apache-2.0 project that people point at production
Dell PowerScale clusters and build dashboards, alerts and recording rules on. Version
1.0.0 should say what will not break without a major bump. It must not promise what the
project does not control: the exporter only reads what OneFS returns, through Dell's
`gopowerscale` SDK, and a few metrics have never been seen on a real cluster.

The policy applies in full from 1.0.0. v0.90.0 is the release before it, with the same
surface; the Provisional tier below is what separates the two.

## Decision

### Promised in v1

Breaking any of these needs a major version bump:

- **Names, label names and units** of every stable `powerscale_*` metric. Adding a label
  to an existing series is breaking: it changes series identity for dashboards and alerts.
- **CLI flags**, **`config.yaml` keys**, the `${VAR}` and `${VAR:-default}` semantics
  (`internal/utils/env.go`), and the default **port 9444**.
- The **top-level keys of the `/health` JSON body** as of 1.0.0 (see ADR-0005). `/livez`
  and `/readyz` always answer 200.
- The **`values.yaml` keys** of the Helm chart.

Not breaking: a new metric, a new label *value*, a new optional config key or flag.

### Not promised: bounded by Dell

- The **presence and value** of a metric depend on the cluster's OneFS version and API
  version. Quota API v1 has no `fslogical`/`fsphysical`; 9.15 adds fields. A metric that is
  absent or zero on an older cluster is not a bug in the exporter.
- Dell can change an endpoint, a stat key or its units in a later OneFS release. We fix
  forward and record it in the CHANGELOG. That is not a break of this policy.
- Upgrades of the `gopowerscale` SDK follow the same rule as any dependency.
- **Dell dashboard compatibility** is best effort. The `powerscale_` prefix matches
  `dell/csm-metrics-powerscale` (ADR-0004); if Dell renames theirs, this project does not
  follow automatically.

### Provisional tier (outside the guarantee)

Each is flagged **Provisional** in `docs/metrics.md`:

- `powerscale_quota_reduction_ratio`: OneFS 9.15, derived from the spec, never seen live.
- `powerscale_protocol_latency_stage_seconds`: OneFS 9.15, same.
- `powerscale_quota_application_usage_bytes`: `usage.applogical`, fixtures only.
- The node hardware block, as a unit: `powerscale_node_power_supplies`,
  `powerscale_node_power_supply_failures`, `powerscale_node_temperature_celsius`,
  `powerscale_node_fan_speed_rpm`, `powerscale_node_hardware_info`.

A provisional metric may change name, labels or unit in a minor release. It becomes stable
after a live capture (`--once --debug --trace --dump-dir`) confirms its presence, unit and
null behaviour; that promotion is a minor release.

### Naming audit before 1.0.0

Metric and label names were audited before this policy took effect. Five count gauges
carried a `_total` suffix, which Prometheus reserves for counters, and were renamed (see the
CHANGELOG, **BREAKING**). The Prometheus linter (the one behind `promtool check metrics`, run in the test suite) also
flagged three duration metrics in microseconds; they now use the base unit, seconds. This
amends the unit list in ADR-0004, which named `_microseconds`. Label names were reviewed (by reading the code and `docs/metrics.md`; `promtool check metrics` was not run) and kept as they are: the `quota_`
prefix on quota labels keeps them distinct when series are joined, and `type` and `node`
are unambiguous within their metric family.

## Consequences

- Users can rely on the stable names and labels across the 1.x line.
- The cost is real: a naming mistake found later waits for 2.0.0, or is added as a new
  metric next to the old one.
- Contributors adding a metric choose its name and labels once; `CONTRIBUTING.md` points
  here.
- Provisional metrics keep honest documentation instead of an implied guarantee.
