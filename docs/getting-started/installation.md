# Installation

The exporter ships as a single static binary and as a multi-arch container image. Pick
whichever fits your environment.

## OneFS account

Collection only needs a **read-only** OneFS account — a role with `ISI_PRIV_STATISTICS`
and `ISI_PRIV_QUOTA` is sufficient. Create a dedicated monitoring user rather than reusing
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
  -p 2112:2112 \
  -v "$PWD/config.yaml:/etc/pscale_exporter/config.yaml:ro" \
  -e PSCALE1_PASSWORD='your-monitor-password' \
  ghcr.io/fjacquet/pscale_exporter:latest
```

The image runs as a non-root user (`uid 10001`) and exposes port `2112`.

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
| `make tools` | Install pinned `golangci-lint`, `cyclonedx-gomod`, `govulncheck`. |
| `make sbom` | Generate a CycloneDX SBOM. |
| `make release` | Cross-compile binaries + SBOM + checksums. |

## Release binaries

Tagged releases publish cross-compiled binaries, an SBOM, and checksums to the
[GitHub Releases](https://github.com/fjacquet/pscale_exporter/releases) page. Download the
archive for your platform, verify the checksum, and place the binary on your `PATH`.

## Verify it's running

```bash
curl -s http://localhost:2112/metrics | grep '^powerscale_up'
curl -s http://localhost:2112/health
```

Next: [Configuration](configuration.md) · [Quick Start](quickstart.md).
