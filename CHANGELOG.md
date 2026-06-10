# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-06-10

### Added

- **Remote-debug observability** — designed for diagnosing clusters we never have direct
  access to:
    - `--debug` now logs every OneFS request (full URL, parameter count, response size,
      duration), a per-cluster inventory parse summary (node/quota/sensor/policy/event
      counts), and a statistics summary naming any curated stat keys the cluster did
      **not** return.
    - Parse failures (required and best-effort endpoints) log a bounded payload snippet
      showing what the API actually returned.
    - The lenient sensor decoders no longer fall back silently: an unrecognized
      `sensors` shape or unparseable sensor value is traced at debug level.
    - New `--dump-dir DIR` flag writes every raw API response verbatim to
      `DIR/<cluster>/<endpoint>.json` (`0600`; bodies carry no credentials) — operators
      ship the directory back and the files drop straight into `testdata/` as fixtures.
- New **Troubleshooting** docs page covering the debug workflow.

## [0.5.4] - 2026-06-10

### Fixed

- **Nodes-payload parsing now matches live OneFS 9.x clusters** (validated against a
  OneFS 9.13 virtual PowerScale), while remaining compatible with the older shapes:
  `sensors` may be an object wrapping a nested `sensors` array, `state.smartfail`
  may carry per-condition booleans (`smartfailed`) instead of a state string, and
  empty drive bays (`"present": false`) are no longer counted as drives.

### Changed

- Docs: the read-only collection role also needs `ISI_PRIV_DEVICES` read access —
  the cluster-nodes inventory endpoint refuses without it.

## [0.5.3] - 2026-06-05

### Changed

- Bump `goreleaser/goreleaser-action` v6.4.0 → **v7.2.2** (Node 24 runtime), clearing the
  GitHub Actions Node 20 deprecation warning. No change to how the action is invoked.

## [0.5.2] - 2026-06-05

### Changed

- **Homebrew cask is now macOS-only.** Builds and archives are split by OS so the cask
  references a darwin-only archive; the generated cask no longer emits unusable `linux`
  URLs (Homebrew casks don't run on Linux). The GitHub Release still ships all four
  archives + SBOMs, and the GHCR image is unchanged. Docs clarified accordingly.

## [0.5.1] - 2026-06-05

> The `v0.5.0` tag was cut first but its release run failed before publishing anything
> (a `dockers_v2` `COPY` path bug); `v0.5.1` is the first published release of this work.

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

[Unreleased]: https://github.com/fjacquet/pscale_exporter/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/fjacquet/pscale_exporter/compare/v0.5.4...v0.6.0
[0.5.4]: https://github.com/fjacquet/pscale_exporter/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/fjacquet/pscale_exporter/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/fjacquet/pscale_exporter/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/fjacquet/pscale_exporter/compare/v0.4.2...v0.5.1
[0.4.2]: https://github.com/fjacquet/pscale_exporter/releases/tag/v0.4.2
