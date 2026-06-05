# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **GoReleaser release pipeline** (`.goreleaser.yaml`). A single tool now produces the
  cross-platform binaries (`linux`/`darwin` × `amd64`/`arm64`), `tar.gz` archives, a
  `checksums.txt`, a per-archive **CycloneDX SBOM** (syft), and the multi-arch GHCR image —
  replacing the bespoke `make release` loop and the separate `docker/build-push-action` job.
- **Homebrew cask distribution** via `github.com/fjacquet/homebrew-tap`:
  `brew install fjacquet/tap/pscale_exporter`. The post-install hook strips the macOS
  quarantine bit so the unsigned binary runs without a Gatekeeper prompt.
- **`LICENSE`** file (Apache-2.0). The README already declared Apache-2.0 but the file was
  missing; it is now present and bundled into every release archive.
- **Changelog** (this file) and an **Architecture Decision Record** documenting the release
  and supply-chain design ([ADR-0001](https://github.com/fjacquet/pscale_exporter/blob/main/docs/adr/0001-release-pipeline-and-supply-chain.md)).

### Changed

- **All GitHub Actions are pinned to full commit SHAs** (with a `# vX.Y.Z` comment) across
  `ci.yml`, `release.yml`, and `docs.yml`. The Semgrep CI container is pinned by image
  digest. This closes the mutable-tag supply-chain vector; Dependabot can still propose bumps.
- **Release is GoReleaser-driven.** `make release` now runs `goreleaser release --clean`;
  `make release-snapshot` runs the full pipeline locally minus publish. `make tools` installs
  a pinned GoReleaser.
- **Image SBOM + provenance** are still attached to the GHCR image — now produced by
  GoReleaser's `dockers_v2` builder (both on by default) instead of `build-push-action`.
- **Go toolchain bumped to 1.26.4** — the `Dockerfile` builder image now matches the
  `go 1.26.4` directive already in `go.mod`.

## [0.4.2] - 2026-06-05

- Baseline prior to the release-pipeline rework. See the
  [GitHub releases](https://github.com/fjacquet/pscale_exporter/releases) for earlier history.

[Unreleased]: https://github.com/fjacquet/pscale_exporter/compare/v0.4.2...HEAD
[0.4.2]: https://github.com/fjacquet/pscale_exporter/releases/tag/v0.4.2
