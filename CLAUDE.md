# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Everything CI runs is a Makefile target, so it reproduces locally.

- `make cli` — build `bin/pscale_exporter` (injects `main.version` via ldflags).
- `make test` / `go test ./...` — tests; `make test-race` adds `-race` + coverage.
- Run a single test: `go test ./internal/powerscale/ -run TestCollect` (most logic lives in the `powerscale` package).
- `make ci` — the full gate: gofmt check, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`.
- `make sure` — fmt + vet + test + build + lint (local convenience).
- `make tools` — install pinned `golangci-lint`, `cyclonedx-gomod`, `govulncheck`, `goreleaser`.
- `make sbom` — CycloneDX SBOM (module-level, used by CI). `make release` — `goreleaser release --clean` (binaries, archives, checksums, per-archive syft SBOM, multi-arch GHCR image, Homebrew cask) driven by `.goreleaser.yaml`. `make release-snapshot` — the full GoReleaser pipeline locally, minus publish.
- `make docker` — from-source image build (`pscale_exporter:$(VERSION)` + `:latest`) using the root `Dockerfile`.
- Run it: `./bin/pscale_exporter --config config.yaml [--debug] [--once] [--trace] [--dump-dir DIR]`. Cluster secrets are `${ENV_VAR}` references in `config.yaml` (or `passwordFile`); export e.g. `PSCALE1_PASSWORD` before running.
- Live-cluster validation: `--once --debug` prints every collected sample sorted in exposition style (diff against `docs/metrics.md`); `--trace` logs every OneFS API response body. **Token safety:** trace logs method/URL/status/body only, never headers (OneFS session credentials live in headers); the `/session/1/session` login happens inside gopowerscale and is structurally excluded. Never enable the SDK's own verbose logging (`GOISILON_DEBUG` / `verboseLogging`): it dumps response headers including `Set-Cookie: isisessid=…` unmasked.
- Docs site (MkDocs Material): `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict` (or `serve`).
- `vendor/` is git-ignored; dependencies are managed with `go mod`.

## Architecture

A Go exporter for **Dell PowerScale (OneFS)** that exposes metrics via **both** a Prometheus `/metrics` endpoint **and** an optional OTLP metric push.

**Snapshot model (the central design choice).** A single background **collection loop** (`internal/powerscale/collector.go`) polls every configured cluster on `collection.interval` and publishes an immutable **snapshot** to a `SnapshotStore` (`snapshot.go`, RWMutex pointer-swap). Both export paths read the latest snapshot rather than fetching on scrape: `PromCollector` (`prometheus.go`, an *unchecked* collector — `Describe` sends nothing so it can emit a dynamic metric-name set) and `OTLPExporter` (`otlp.go`, asynchronous observable gauges driven by a periodic reader). This decouples OneFS API load from the number of scrapers and the OTLP push cadence. `main.go` wires the HTTP server, the loop, hot config reload, and `/health` (snapshot-based).

**Per-cluster client (`client.go`).** One gopowerscale `api.Client` session per cluster, created once via `api.New` (which negotiates the platform API version). That single session serves **both**:
- **typed resources** — cluster config/identity, cluster nodes, quotas, and NFS/SMB/snapshot counts; and
- **the raw statistics API** — `platform/1/statistics/current` for the curated stat keys plus `platform/2/statistics/summary/protocol` for the per-protocol summary.

**API-version detection** delegates to the SDK's `APIVersion()` (negotiated at `api.New`); `collector.go` reads it per cluster and stores it on the snapshot.

**Graceful degradation.** If a cluster's session can't be created or a collection fails, that cluster is logged as init-failed / marked down on the snapshot; the exporter and other clusters keep running.

**Stat-key mapping (`statkeys.go` + `statisticsKeys.json`, sample derivation `derivations.go`, types `metrics.go`).** A curated JSON table maps OneFS stat keys to metric names and scope; node-scope keys map a `devid` to a node LNN. Samples are `Sample{Name, []Label, Value}` with `cluster`/`cluster_id` as the canonical leading labels.

## Conventions and non-obvious constraints

- **`powerscale_` metric prefix** — matches `dell/csm-metrics-powerscale` for dashboard compatibility. Keep it.
- **`iops` and bandwidth are per-second gauges** — in PromQL aggregate with `sum`/`avg`, never `rate()`.
- **Unit-explicit names:** metric names carry their unit — `_bytes`, `_bytes_per_second`, `_operations_per_second`, `_microseconds`, `_percent`.
- **Extending coverage:** add a row to `internal/powerscale/statisticsKeys.json` (`key`, `metric`, `scope` = `cluster` | `node`). No code change is needed for a new curated key. Node-scope keys map `devid` → node LNN.
- **Semgrep write-hook blocks on findings and inline `// nosemgrep` is NOT honored** — fix by restructuring, not suppression (e.g. test HTTP handlers write fixtures through a `writeBytes(io.Writer, …)` helper to avoid the "write-to-ResponseWriter" rule). The **Dockerfile must declare a non-root `USER`**.
- **Two Dockerfiles, different jobs:** the root `Dockerfile` is the from-source build used by `make docker`; `Dockerfile.goreleaser` is runtime-only (GoReleaser stages the prebuilt binary) and used by the release pipeline. Edit the one matching the build path you're changing; keep both non-root.

## Testing

`internal/powerscale/mockserver_test.go` runs a mock OneFS platform-API server (`httptest`) that serves the session, cluster config/nodes, quotas, NFS/SMB/snapshot, and the `/statistics/current` + `/statistics/summary/protocol` endpoints from `testdata/` fixtures. Collector tests assert results via **both** the Prometheus registry gather and an OTLP `ManualReader`; `e2e_test.go` exercises the full collect → snapshot → export path.

## CI/CD

`.github/workflows/`: `ci.yml` (`make ci` + SBOM artifact + Semgrep), `release.yml` (on `v*` tags: a single **GoReleaser** job — binaries, archives, checksums, per-archive syft SBOM to a GitHub Release, a multi-arch GHCR image with SBOM/provenance attestations via `dockers_v2`, and a Homebrew cask to `fjacquet/homebrew-tap`), `docs.yml` (MkDocs → GitHub Pages). **Every action is pinned to a full commit SHA** with a `# vX.Y.Z` comment; the Semgrep CI container is pinned by digest. When bumping a pin, resolve the new tag to its SHA first (`gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`) and keep the comment in sync. The release needs a `HOMEBREW_TAP_GITHUB_TOKEN` secret (PAT with `contents:write` on the tap repo) for the cask step. Design rationale: `docs/adr/0001-release-pipeline-and-supply-chain.md`.

> A Semgrep scan runs on every file write via a hook and **blocks on findings**. Inline `// nosemgrep` is **not** honored — fix by restructuring, not suppression. Dockerfiles must declare a non-root `USER`.
