# Installation

The exporter ships as a single static binary and as a multi-arch container image. Pick
whichever fits your environment.

## OneFS account

Collection only needs a **read-only** OneFS account. Grant its role read access to
`ISI_PRIV_STATISTICS`, `ISI_PRIV_QUOTA`, `ISI_PRIV_DEVICES` (the node inventory endpoint
refuses without it), `ISI_PRIV_EVENT`, `ISI_PRIV_SNAPSHOT`, `ISI_PRIV_SYNCIQ`,
`ISI_PRIV_SMB`, `ISI_PRIV_NFS`, `ISI_PRIV_LICENSE`, and `ISI_PRIV_SMARTPOOLS`. Create a dedicated monitoring user rather than reusing
an admin account.

## Container image (recommended)

Images are published to GitHub Container Registry for `linux/amd64` and `linux/arm64`:

```bash
docker pull ghcr.io/fjacquet/pscale_exporter:latest
```

Tags follow the release tags (`vX.Y.Z`) plus `latest`. Each image carries an SBOM and
provenance attestations (see [CI/CD & SBOM](../cicd.md)).

Run it with your config mounted and the cluster secret in the environment:

```bash
docker run --rm \
  -p 9444:9444 \
  -v "$PWD/config.yaml:/etc/pscale_exporter/config.yaml:ro" \
  -e PSCALE1_PASSWORD='your-monitor-password' \
  ghcr.io/fjacquet/pscale_exporter:latest
```

The image runs as a non-root user (`uid 10001`) and exposes port `9444`.

## Homebrew (macOS)

The exporter is distributed as a **cask** (a prebuilt binary, not built from source).
Casks are macOS-only — on Linux, use the [release tarball](#release-binaries) or the
[container image](#container-image-recommended) instead.

Install the CLI from the project tap:

```bash
brew install fjacquet/tap/pscale_exporter
```

That is shorthand for tapping first, if you prefer to see the tap explicitly:

```bash
brew tap fjacquet/tap
brew install pscale_exporter
```

Then run it with your config and the cluster secret in the environment:

```bash
export PSCALE1_PASSWORD='your-monitor-password'
pscale_exporter --config config.yaml
```

Upgrade and uninstall as usual:

```bash
brew upgrade pscale_exporter
brew uninstall pscale_exporter
```

!!! note "Unsigned binary"
    The cask is not Apple code-signed; its install hook clears the macOS quarantine bit so
    Gatekeeper won't block it. If you ever hit a quarantine prompt, run
    `xattr -dr com.apple.quarantine "$(command -v pscale_exporter)"`.

## Build from source

Requires the Go toolchain pinned in `go.mod`.

```bash
make cli                       # builds bin/pscale_exporter (injects main.version via ldflags)
export PSCALE1_PASSWORD='your-monitor-password'
./bin/pscale_exporter --config config.yaml
```

Other useful targets:

| Target | What it does |
|---|---|
| `make cli` | Build `bin/pscale_exporter`. |
| `make test` / `make test-race` | Run tests (the latter adds `-race` + coverage). |
| `make sure` | `fmt` + `vet` + `test` + `build` + `golangci-lint` (local convenience). |
| `make ci` | The full CI gate: gofmt check, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`. |
| `make tools` | Install pinned `golangci-lint`, `cyclonedx-gomod`, `govulncheck`, `goreleaser`. |
| `make sbom` | Generate a CycloneDX SBOM. |
| `make release-snapshot` | Run the full GoReleaser pipeline locally, minus publish. |

## Release binaries

Tagged releases publish, via GoReleaser, `tar.gz` archives (per platform), a per-archive
CycloneDX SBOM, and `checksums.txt` to the
[GitHub Releases](https://github.com/fjacquet/pscale_exporter/releases) page. Download the
archive for your platform, verify it against `checksums.txt`, and place the binary on your
`PATH`.

## Verify it's running

```bash
curl -s http://localhost:9444/metrics | grep '^powerscale_up'
curl -s http://localhost:9444/health
```

Next: [Configuration](configuration.md) · [Quick Start](quickstart.md).
