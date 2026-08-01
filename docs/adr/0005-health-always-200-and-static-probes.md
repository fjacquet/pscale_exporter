# `/livez` `/readyz`, and `/health` always answering 200

## Status

Accepted (2026-08-01)

## Context

Same argument as obs_exporter's ADR-0013 and ADR-0014, applied here in one
pass: an exporter is a probe. "Cluster unreachable" is data it reports, not
a failure of the exporter process. Coupling that fact to an HTTP status code
on any endpoint — the chart's `livenessProbe`/`readinessProbe`, or the
informational `/health` — risks something downstream (kubelet, a dashboard,
a script) treating a healthy, correctly-reporting exporter as down.

`charts/pscale-exporter/values.yaml` wired both `livenessProbe` and
`readinessProbe` to `/health`, which answered 503 when every configured
cluster was unreachable. As a *liveness* check this was always wrong: no
restart makes an unreachable cluster reachable.

## Decision

Two new endpoints, `/livez` and `/readyz`, both `staticOKHandler` — always
`200 OK`, no `SnapshotStore` read, nothing that can make either fail once
the process is running. The chart's default probes now point at them.

`/health`'s `healthHandler` no longer writes plain text or a 503. It always
answers 200 with a JSON body: `clusters: [{cluster, ok, last_scrape, err}]`,
built from the same `SnapshotStore` `/metrics` reads.

## Consequences

- **Breaking**: `/health`'s response body changes from plain text
  (`"OK"`/`"OK (starting)"`/`"UNHEALTHY: ..."`) to JSON, and its status code
  is always 200 (previously 503 when every cluster was down). Anything
  parsing the old text format or gating on the old status code needs
  updating.
- Chart default probe wiring changes; a fresh `helm install` or an upgrade
  without pinned probe overrides gets the fix automatically.
- Alert on a per-cluster `_up` metric (or `/health`'s body), never on any
  probe's HTTP status.
