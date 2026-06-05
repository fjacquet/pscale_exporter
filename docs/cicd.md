# CI/CD & SBOM

Everything CI runs is a Makefile target, so it reproduces locally.

## Workflows

### `ci.yml` — on every push to `main` and every PR

- **quality** — installs Go from `go.mod`, runs `make tools`, then `make ci`: gofmt check,
  `go vet`, `golangci-lint`, `go test -race` (with coverage), and `govulncheck`. Coverage
  is uploaded as an artifact.
- **sbom** — `make sbom` produces a CycloneDX SBOM artifact.
- **semgrep** — static security analysis.

Reproduce the gate locally:

```bash
make tools   # pinned golangci-lint, cyclonedx-gomod, govulncheck
make ci      # gofmt + vet + lint + go test -race + govulncheck
make sure    # local convenience: fmt + vet + test + build + lint
```

### `release.yml` — on `v*` tags

A single **GoReleaser** job (`.goreleaser.yaml`) produces, from one `git tag`:

- cross-platform binaries (`linux`/`darwin` × `amd64`/`arm64`), `tar.gz` archives, and
  `checksums.txt`, published to a GitHub Release;
- a per-archive **CycloneDX SBOM** (syft);
- the multi-arch (`linux/amd64`, `linux/arm64`) image pushed to
  `ghcr.io/fjacquet/pscale_exporter`, with **SBOM and provenance attestations** (GoReleaser's
  `dockers_v2` builder enables both by default);
- a **Homebrew cask** pushed to `github.com/fjacquet/homebrew-tap`
  (`brew install fjacquet/tap/pscale_exporter`).

```bash
make release-snapshot   # full pipeline locally, minus publish (no Release, no image push)
make release            # goreleaser release --clean (what CI runs)
```

!!! note "Required secret"
    The cask step pushes to a *different* repo, so it needs a `HOMEBREW_TAP_GITHUB_TOKEN`
    secret (a PAT with `contents:write` on `homebrew-tap`); the default `GITHUB_TOKEN`
    cannot push cross-repo. The rest of the release works without it.

### `docs.yml` — on changes to `docs/`, `mkdocs.yml`, or the workflow

Builds this MkDocs Material site with `--strict` and deploys it to GitHub Pages. Build it
locally exactly as CI does:

```bash
uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict
# or: uvx --with mkdocs-material --with pymdown-extensions mkdocs serve
```

## Security constraints

!!! warning "Semgrep write-hook"
    A Semgrep scan runs on every file write and **blocks on findings**. Inline
    `// nosemgrep` is **not** honored — fix by restructuring, not suppression. Dockerfiles
    must declare a non-root `USER` (the runtime image runs as `uid 10001`).

## Supply chain

Each release carries a CycloneDX SBOM (binaries and image) plus build provenance
attestations on the GHCR image, so consumers can verify what's inside an artifact.

Every GitHub Action is **pinned to a full commit SHA** (with a `# vX.Y.Z` comment) and the
Semgrep CI container is pinned by image digest, so a moving or compromised upstream tag can't
silently run with the workflows' write permissions. Dependabot still proposes version bumps
against the pins. The rationale is recorded in
[ADR-0001](adr/0001-release-pipeline-and-supply-chain.md).
