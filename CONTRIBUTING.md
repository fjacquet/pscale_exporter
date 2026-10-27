# Contributing to pscale_exporter

Thank you for contributing. This guide covers the local workflow, the rules to respect and
the pull request checklist.

## Prerequisites

| Tool | Version | How to get |
|------|---------|------------|
| Go | see `go.mod` | <https://go.dev/dl/> |
| golangci-lint, govulncheck, cyclonedx-gomod, goreleaser | pinned in the Makefile | `make tools` |
| mkdocs-material | latest | `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict` |
| Docker + Compose | any recent | only for the local stack |

## Local workflow

```bash
make cli        # build bin/pscale_exporter
make test       # go test ./...
make ci         # the gate CI runs: gofmt check, vet, golangci-lint, go test -race, govulncheck
make sure       # local convenience: fmt + vet + test + build + lint
```

Run a single test: `go test ./internal/powerscale/ -run TestCollect`.

## Tests

New features and bug fixes include tests. `internal/powerscale/mockserver_test.go` is a mock
OneFS platform-API server driven by fixtures in `testdata/`. Collector tests assert through
both the Prometheus registry and an OTLP `ManualReader`. Fixtures must match the real API:
`make schemas` regenerates the schema snapshot that a guard test checks them against.

## Adding or changing a metric

Names, label names and units are a public interface: read the
[stability policy](docs/adr/0007-v1-stability-policy.md) first.

- Keep the `powerscale_` prefix and a unit-explicit name (`_bytes`, `_bytes_per_second`,
  `_operations_per_second`, `_seconds`, `_percent`). Do not use `_total` for gauges, and give durations in seconds.
- `iops` and bandwidth are per-second gauges; document that users must not `rate()` them.
- For a new curated OneFS stat key, add a row to `internal/powerscale/statisticsKeys.json`
  (`key`, `metric`, `scope`, optional `divisor`); no code change is needed. Verify the unit
  against a real cluster, because the API docs do not state it.
- A metric never seen on a live cluster is marked **Provisional** in `docs/metrics.md`.
- Document the metric in `docs/metrics.md`.

## Code rules

- No inline suppressions: no `//nolint` and no `// nosemgrep`. The Semgrep gate does not honor
  them; restructure the code instead.
- The Dockerfiles must declare a non-root `USER`. The root `Dockerfile` is the from-source
  build; `Dockerfile.goreleaser` is runtime-only.
- GitHub Actions are pinned to a full commit SHA with a `# vX.Y.Z` comment.

## Commits and pull requests

- [Conventional Commits](https://www.conventionalcommits.org/): `feat(quota): ...`,
  `fix(...)`, `docs(...)`, `test(...)`, `refactor(...)`, `ci(...)`, `chore(...)`.
- Merges are **merge commits**; squash is disabled on the repository.
- Significant, hard-to-reverse decisions get an ADR in `docs/adr/`.

### Checklist

- [ ] `make ci` passes locally
- [ ] Tests added or updated
- [ ] New or changed metrics are in `docs/metrics.md`
- [ ] A user-visible change has a `CHANGELOG.md` entry (mark breaking changes **BREAKING**)
- [ ] No inline suppressions
- [ ] `mkdocs build --strict` passes if docs changed

## Reporting a bug

Open an issue with the exporter version, the OneFS version and what you expected. For a
metric that is missing or looks wrong, attach the output of:

```bash
pscale_exporter --config config.yaml --once --debug --trace --dump-dir ./capture
```

The capture contains cluster, node, quota path and share names but no credentials; review
and anonymize it before posting. Security problems go through [SECURITY.md](SECURITY.md),
not a public issue.
