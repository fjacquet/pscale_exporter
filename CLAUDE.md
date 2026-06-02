# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Everything CI runs is a Makefile target, so it reproduces locally.

- `make cli` — build `bin/pscale_exporter` (injects `main.version` via ldflags).
- `make test` / `go test ./...` — tests; `make test-race` adds `-race` + coverage.
- Run a single test: `go test ./internal/powerscale/ -run TestCollect` (most logic lives in the `powerscale` package).
- `make ci` — the full gate: gofmt check, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`.
- `make sure` — fmt + vet + test + build + lint (local convenience).
- `make tools` — install pinned `golangci-lint`, `cyclonedx-gomod`, `govulncheck`.
- `make sbom` — CycloneDX SBOM; `make release` — cross-compile binaries + SBOM + checksums.
- Run it: `./bin/pscale_exporter --config config.yaml [--debug] [--once]`. Cluster secrets are `${ENV_VAR}` references in `config.yaml` (or `passwordFile`); export e.g. `PSCALE1_PASSWORD` before running.
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

## Testing

`internal/powerscale/mockserver_test.go` runs a mock OneFS platform-API server (`httptest`) that serves the session, cluster config/nodes, quotas, NFS/SMB/snapshot, and the `/statistics/current` + `/statistics/summary/protocol` endpoints from `testdata/` fixtures. Collector tests assert results via **both** the Prometheus registry gather and an OTLP `ManualReader`; `e2e_test.go` exercises the full collect → snapshot → export path.

## CI/CD

`.github/workflows/`: `ci.yml` (`make ci` + SBOM artifact + Semgrep), `release.yml` (on `v*` tags: binaries + SBOM to a GitHub Release, plus a multi-arch GHCR image with SBOM/provenance attestations), `docs.yml` (MkDocs → GitHub Pages). Actions are pinned to current Node 24-major runtimes. When adding or bumping a workflow action, confirm the tag exists first.

> A Semgrep scan runs on every file write via a hook and **blocks on findings**. Inline `// nosemgrep` is **not** honored — fix by restructuring, not suppression. Dockerfiles must declare a non-root `USER`.

<!-- rtk-instructions v2 -->
# RTK (Rust Token Killer) - Token-Optimized Commands

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:

```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands by Workflow

### Build & Compile (80-90% savings)

```bash
rtk cargo build         # Cargo build output
rtk cargo check         # Cargo check output
rtk cargo clippy        # Clippy warnings grouped by file (80%)
rtk tsc                 # TypeScript errors grouped by file/code (83%)
rtk lint                # ESLint/Biome violations grouped (84%)
rtk prettier --check    # Files needing format only (70%)
rtk next build          # Next.js build with route metrics (87%)
```

### Test (60-99% savings)

```bash
rtk cargo test          # Cargo test failures only (90%)
rtk go test             # Go test failures only (90%)
rtk jest                # Jest failures only (99.5%)
rtk vitest              # Vitest failures only (99.5%)
rtk playwright test     # Playwright failures only (94%)
rtk pytest              # Python test failures only (90%)
rtk rake test           # Ruby test failures only (90%)
rtk rspec               # RSpec test failures only (60%)
rtk test <cmd>          # Generic test wrapper - failures only
```

### Git (59-80% savings)

```bash
rtk git status          # Compact status
rtk git log             # Compact log (works with all git flags)
rtk git diff            # Compact diff (80%)
rtk git show            # Compact show (80%)
rtk git add             # Ultra-compact confirmations (59%)
rtk git commit          # Ultra-compact confirmations (59%)
rtk git push            # Ultra-compact confirmations
rtk git pull            # Ultra-compact confirmations
rtk git branch          # Compact branch list
rtk git fetch           # Compact fetch
rtk git stash           # Compact stash
rtk git worktree        # Compact worktree
```

Note: Git passthrough works for ALL subcommands, even those not explicitly listed.

### GitHub (26-87% savings)

```bash
rtk gh pr view <num>    # Compact PR view (87%)
rtk gh pr checks        # Compact PR checks (79%)
rtk gh run list         # Compact workflow runs (82%)
rtk gh issue list       # Compact issue list (80%)
rtk gh api              # Compact API responses (26%)
```

### JavaScript/TypeScript Tooling (70-90% savings)

```bash
rtk pnpm list           # Compact dependency tree (70%)
rtk pnpm outdated       # Compact outdated packages (80%)
rtk pnpm install        # Compact install output (90%)
rtk npm run <script>    # Compact npm script output
rtk npx <cmd>           # Compact npx command output
rtk prisma              # Prisma without ASCII art (88%)
```

### Files & Search (60-75% savings)

```bash
rtk ls <path>           # Tree format, compact (65%)
rtk read <file>         # Code reading with filtering (60%)
rtk grep <pattern>      # Search grouped by file (75%). Format flags (-c, -l, -L, -o, -Z) run raw.
rtk find <pattern>      # Find grouped by directory (70%)
```

### Analysis & Debug (70-90% savings)

```bash
rtk err <cmd>           # Filter errors only from any command
rtk log <file>          # Deduplicated logs with counts
rtk json <file>         # JSON structure without values
rtk deps                # Dependency overview
rtk env                 # Environment variables compact
rtk summary <cmd>       # Smart summary of command output
rtk diff                # Ultra-compact diffs
```

### Infrastructure (85% savings)

```bash
rtk docker ps           # Compact container list
rtk docker images       # Compact image list
rtk docker logs <c>     # Deduplicated logs
rtk kubectl get         # Compact resource list
rtk kubectl logs        # Deduplicated pod logs
```

### Network (65-70% savings)

```bash
rtk curl <url>          # Compact HTTP responses (70%)
rtk wget <url>          # Compact download output (65%)
```

### Meta Commands

```bash
rtk gain                # View token savings statistics
rtk gain --history      # View command history with savings
rtk discover            # Analyze Claude Code sessions for missed RTK usage
rtk proxy <cmd>         # Run command without filtering (for debugging)
rtk init                # Add RTK instructions to CLAUDE.md
rtk init --global       # Add RTK to ~/.claude/CLAUDE.md
```

## Token Savings Overview

| Category | Commands | Typical Savings |
|----------|----------|-----------------|
| Tests | vitest, playwright, cargo test | 90-99% |
| Build | next, tsc, lint, prettier | 70-87% |
| Git | status, log, diff, add, commit | 59-80% |
| GitHub | gh pr, gh run, gh issue | 26-87% |
| Package Managers | pnpm, npm, npx | 70-90% |
| Files | ls, read, grep, find | 60-75% |
| Infrastructure | docker, kubectl | 85% |
| Network | curl, wget | 65-70% |

Overall average: **60-90% token reduction** on common development operations.
<!-- /rtk-instructions -->
</content>
