# Alpine Standard — Dockerfile.goreleaser HEALTHCHECK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `HEALTHCHECK` to `Dockerfile.goreleaser` — the file that builds pscale_exporter's actually-published image — matching the `/livez` check already added to the local `./Dockerfile` and both compose files on 2026-08-01.

**Architecture:** No base-image change needed — `Dockerfile.goreleaser` is already `alpine:latest`. Purely additive: one `HEALTHCHECK` instruction using the same verified pattern.

**Tech Stack:** Docker, Alpine (`wget`/busybox).

**Spec:** `docs/superpowers/specs/2026-08-01-alpine-standard-design.md` in `obs_exporter` (family-wide design).

## Global Constraints

- `HEALTHCHECK` targets `http://127.0.0.1:9444/livez`, never `localhost` — Alpine's busybox `wget` resolves `localhost` via `::1` first, and the exporter only binds IPv4.
- Timing: `--interval=30s --timeout=5s --start-period=10s --retries=3`.
- Verify by building and running the image, not just by reading the Dockerfile.
- No inline `nosemgrep`/`//nolint` suppressions.

## File Structure

| File | Responsibility |
| --- | --- |
| `Dockerfile.goreleaser` | Adds `HEALTHCHECK` before `USER pscale` |
| `docs/adr/000N-alpine-standard.md` | Records the family decision as it applies to this repo |
| `CHANGELOG.md` | `Added` entry (non-breaking) |

---

### Task 1: Add HEALTHCHECK to Dockerfile.goreleaser

**Files:**
- Modify: `Dockerfile.goreleaser`

**Interfaces:** none.

- [ ] **Step 1: Edit the file**

Insert before `USER pscale`:

```dockerfile
EXPOSE 9444

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9444/livez || exit 1

USER pscale
```

(`EXPOSE 9444` already exists — only the `HEALTHCHECK` block is new.)

- [ ] **Step 2: Lint**

Run: `hadolint Dockerfile.goreleaser`
Expected: no new findings versus the current baseline.

- [ ] **Step 3: Build and verify at runtime**

```bash
CGO_ENABLED=0 go build -o pscale_exporter .
mkdir -p linux/amd64 && cp pscale_exporter linux/amd64/pscale_exporter
docker build -f Dockerfile.goreleaser --build-arg TARGETPLATFORM=linux/amd64 -t pscale_exporter:healthcheck-test .
docker run -d --name pscale-hc-test -p 19444:9444 \
  -e PSCALE1_HOSTNAME=pscale-clu1.example.com -e PSCALE1_USERNAME=pscale-monitor -e PSCALE1_PASSWORD=dummy \
  pscale_exporter:healthcheck-test
sleep 15
docker inspect --format='{{.State.Health.Status}}' pscale-hc-test
```

Expected: `healthy`.

- [ ] **Step 4: Clean up test artifacts**

```bash
docker rm -f pscale-hc-test
docker rmi pscale_exporter:healthcheck-test
rm -rf linux pscale_exporter
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.goreleaser
git commit -m "feat(docker): add HEALTHCHECK to the published image (Dockerfile.goreleaser)"
```

---

### Task 2: ADR + CHANGELOG

**Files:**
- Create: `docs/adr/000N-alpine-standard.md` (N = next free number — check before writing)
- Modify: `CHANGELOG.md`

**Interfaces:** none.

- [ ] **Step 1: Find the next ADR number**

Run: `ls docs/adr/ | sort -V | tail -3`

- [ ] **Step 2: Write the ADR**

```markdown
# Standardize container base image on Alpine, add HEALTHCHECK to the published image

## Status

Accepted (2026-08-01)

## Context

The exporter family had two published-image patterns — Alpine (5 repos, this one
included) and `gcr.io/distroless/static:nonroot` (3 repos) — as undocumented
per-repo author choice, with no written criterion. Alpine has a shell and `wget`,
so it can carry a Docker `HEALTHCHECK`; distroless cannot. pscale_exporter's local
`./Dockerfile` and compose files already got a `/livez`-based `HEALTHCHECK` /
`healthcheck:` on 2026-08-01; `Dockerfile.goreleaser` — the file that builds the
image this repo actually publishes to GHCR — did not.

## Decision

`Dockerfile.goreleaser` gains the same `HEALTHCHECK`, using `127.0.0.1` (not
`localhost` — Alpine's busybox `wget` resolves `localhost` via `::1` first, and
the exporter only binds IPv4) against `/livez`, which never depends on target
reachability or the collection cycle.

## Consequences

- Non-breaking: purely additive. No base-image or UID change (already Alpine,
  already uid 10001).
- The full family standard, including the three distroless repos' conversion, is
  recorded in `obs_exporter`'s
  `docs/superpowers/specs/2026-08-01-alpine-standard-design.md`.
```

- [ ] **Step 3: Add the CHANGELOG entry**

Under `## [Unreleased]` (create it above the most recent version heading if absent), `### Added`:

```markdown
- `HEALTHCHECK` added to the published Docker image, checking `/livez`. See
  ADR-000N.
```

- [ ] **Step 4: Commit**

```bash
git add docs/adr/000N-alpine-standard.md CHANGELOG.md
git commit -m "docs: record ADR-000N (HEALTHCHECK on the published image)"
```

## Self-Review

- Spec coverage: this repo's row in the family table (`Dockerfile.goreleaser` HEALTHCHECK only, no base-image change) — covered by Task 1. Documentation — covered by Task 2.
- No placeholders: ADR number requires a one-command check (Step 1) before writing.
- Scope: single repo, two tasks, matches the family plan's per-repo row exactly.
