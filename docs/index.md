# pscale_exporter

A Go exporter for Dell PowerScale (OneFS) clusters. Authenticates to the OneFS platform
API, collects broad cluster/node/protocol/quota/capacity metrics, and exposes them via a
Prometheus `/metrics` endpoint and an optional OTLP metric push.

See the [design spec](superpowers/specs/2026-06-02-powerscale-exporter-design.md).
