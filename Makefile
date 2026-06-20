BIN     = pscale_exporter
DIST    = dist
COVER   ?= coverage.out
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

# Pinned tool versions (installed by `make tools`).
GOLANGCI_LINT_VERSION   ?= v2.12.2
CYCLONEDX_GOMOD_VERSION ?= latest
GOVULNCHECK_VERSION     ?= latest
GORELEASER_VERSION      ?= v2.16.0

all: cli test docker

# Install pinned dev/CI tooling into $(GOBIN)/$GOPATH/bin.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

# --- quality gates (used by CI) ---

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed in:"; gofmt -l .; exit 1)

fmt:
	go fmt ./...

# Canonical alias for golangci-lint fmt (matches fjacquet/ci interface).
format:
	golangci-lint fmt

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test -race -coverprofile=$(COVER) -covermode=atomic ./...

test-race:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

test-coverage: test-race
	go tool cover -html=coverage.out -o coverage.html

vuln:
	govulncheck ./...

# Canonical alias: go build for compile-check (matches fjacquet/ci interface).
build:
	go build -v ./...

# Canonical alias: download module dependencies.
install:
	go mod download

# Semgrep SAST scan (matches fjacquet/ci interface; uvx provided by CI setup-uv step).
security:  # advisory: reports findings but never blocks the build (CodeQL/osv are the blocking gates)
	uvx semgrep scan --config auto --skip-unknown-extensions || true

# Upload coverage to Codecov (matches fjacquet/ci interface).
coverage-upload:
	uvx --from codecov-cli codecov upload-process --file $(COVER) || true

# Build MkDocs site (matches fjacquet/ci interface).
docs:
	uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict --site-dir site

# Aggregate gate run by CI.
ci: fmt-check vet lint test-race vuln

# Local convenience: format, vet, test, build, lint.
sure: fmt vet test
	go build ./...
	golangci-lint run

# --- artifacts ---

cli:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BIN) .

# CycloneDX SBOM for the Go module (source/dependency SBOM).
sbom:
	@mkdir -p $(DIST)
	cyclonedx-gomod mod -licenses -json -output $(DIST)/sbom.cdx.json
	@echo "wrote $(DIST)/sbom.cdx.json"

# Tag-triggered release: GoReleaser builds binaries, archives, checksums,
# per-archive CycloneDX SBOMs, and the multi-arch GHCR image (run by CI).
release:
	goreleaser release --clean

# Local dry run: full pipeline minus publish (no GitHub Release, no image push).
release-snapshot:
	goreleaser release --snapshot --clean --skip=publish

docker:
	docker build -t $(BIN):$(VERSION) -t $(BIN):latest .

run-cli: cli
	./bin/$(BIN) --config config.yaml

clean-dist:
	rm -rf $(DIST)

clean: clean-dist
	rm -f bin/$(BIN) coverage.out coverage.html

.PHONY: schemas
schemas: ## Regenerate testdata/onefs_schemas.json from docs/swagger/<spec>.json
	go run ./tools/extract-schemas

.PHONY: all tools fmt-check fmt format vet lint test test-race test-coverage vuln \
        build install security coverage-upload docs ci sure \
        cli sbom release release-snapshot docker run-cli clean-dist clean schemas
