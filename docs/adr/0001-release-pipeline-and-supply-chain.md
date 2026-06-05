# ADR-0001: GoReleaser release pipeline, SHA-pinned actions, and supply-chain hardening

- **Status:** Accepted
- **Date:** 2026-06-05
- **Deciders:** Frederic Jacquet

## Context

The release path had grown organically and carried three weaknesses:

1. **Bespoke release tooling.** `make release` cross-compiled binaries with a hand-written
   shell loop and `sha256sum`, then `softprops/action-gh-release` uploaded them. A *separate*
   `docker/build-push-action` job built the GHCR image. SBOMs came from `cyclonedx-gomod`.
   Three tools, two jobs, two SBOM formats, and behaviour that only existed in CI — hard to
   reproduce locally and easy to drift.
2. **Mutable action tags.** Every GitHub Action was referenced by a moving tag
   (`actions/checkout@v6`, `docker/build-push-action@v7`, …). A compromised or retagged
   action would execute with our `contents: write` / `packages: write` / `id-token: write`
   permissions — a well-known supply-chain vector.
3. **No macOS distribution channel** beyond raw binaries, and a `LICENSE` that the README
   advertised but that did not exist in the repo.

## Decision

**Adopt GoReleaser (v2, `dockers_v2`) as the single release engine** and harden the
supply chain:

- **One config, one tool.** `.goreleaser.yaml` owns binaries, `tar.gz` archives,
  `checksums.txt`, per-archive **CycloneDX SBOMs** (syft), and the multi-arch
  (`linux/amd64`, `linux/arm64`) GHCR image. The image keeps its **SBOM + provenance
  attestations** — now produced by `dockers_v2`, which enables both by default.
- **Pin every GitHub Action to a full commit SHA**, annotated with a `# vX.Y.Z` comment,
  across `ci.yml`, `release.yml`, and `docs.yml`. Pin the Semgrep CI container by image
  digest. Dependabot can still raise version-bump PRs against the pinned SHAs.
- **Distribute via a Homebrew cask** in `github.com/fjacquet/homebrew-tap`
  (`brew install fjacquet/tap/pscale_exporter`). A cross-repo push needs a dedicated
  `HOMEBREW_TAP_GITHUB_TOKEN` PAT, since the workflow's `GITHUB_TOKEN` cannot write to
  another repository.
- **Add the missing Apache-2.0 `LICENSE`** and bundle it into release archives.
- **Bump the Go builder image to 1.26.4** to match the `go.mod` directive.

A dedicated runtime `Dockerfile.goreleaser` (no Go builder stage — GoReleaser stages the
prebuilt binary) is used for images; the root `Dockerfile` remains the from-source build for
`make docker`. Both declare a non-root `USER`, per the project's Semgrep constraint.

### Alternatives considered

- **Keep the bespoke scripts, add SHA-pinning only.** Rejected: leaves the
  three-tool/two-job sprawl and the local/CI drift untouched.
- **GoReleaser `dockers` + `docker_manifests` (the older builder).** Rejected in favour of
  `dockers_v2`, which expresses multi-arch in one block and gives image SBOM/provenance for
  free, removing the per-arch boilerplate.
- **Homebrew formula (`brews:`).** Rejected because GoReleaser has deprecated `brews` in
  favour of `homebrew_casks` for binary distribution.
- **syft for the CI SBOM too.** Deferred: the `ci.yml` SBOM job still uses `cyclonedx-gomod`
  for a fast module-level SBOM on PRs; release artifacts use syft. Revisit if the dual
  tooling becomes a maintenance burden.

## Consequences

**Positive**

- Releases reproduce locally via `make release-snapshot` (full pipeline minus publish).
- Pinned SHAs mean a known, immutable set of third-party code runs with our write tokens.
- One artifact story: binaries, archives, checksums, SBOMs, image, and cask from one `git tag`.
- macOS users get `brew install`.

**Negative / costs**

- A new **`HOMEBREW_TAP_GITHUB_TOKEN`** secret and a `homebrew-tap` repo must exist before the
  cask step succeeds.
- SHA pins are opaque to read; the `# vX.Y.Z` comments and Dependabot mitigate this.
- The release job now needs QEMU + Buildx + syft available (handled by pinned setup actions).
- Two SBOM tools remain in play (cyclonedx-gomod on PRs, syft on releases).

## References

- [Keep a Changelog](https://keepachangelog.com/) — `CHANGELOG.md`
- GoReleaser `dockers_v2`, `homebrew_casks`, and `sboms` customization docs
- GitHub: "Using third-party actions" — pin actions to a full-length commit SHA
