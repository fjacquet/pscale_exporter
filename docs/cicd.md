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

- **binaries** — cross-compiles binaries and publishes them with an SBOM and checksums to
  a GitHub Release.
- **image** — builds a multi-arch (`linux/amd64`, `linux/arm64`) image, pushes it to
  `ghcr.io/fjacquet/pscale_exporter`, and attaches SBOM and provenance attestations.

```bash
make sbom       # CycloneDX SBOM
make release    # cross-compile binaries + SBOM + checksums
```

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
