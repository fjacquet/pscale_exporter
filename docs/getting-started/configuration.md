# Configuration

The exporter reads a single YAML file (default `config.yaml`, override with `--config`).
One process monitors many OneFS clusters.

## Command-line flags

| Flag | Purpose |
| --- | --- |
| `--config <path>` | Path to the config file (default `config.yaml`). |
| `--debug` | Verbose logging. Combined with `--once`, also prints every collected sample (sorted, exposition style). |
| `--once` | Run a single collection cycle and exit (useful for smoke tests / cron). |
| `--trace` | Log every OneFS API response body (method, URL, status; headers — and thus session credentials — are never logged). Very verbose; for live-cluster payload validation. |
| `--dump-dir <dir>` | Write every raw OneFS API response to `<dir>/<cluster>/<endpoint>.json` (offline diagnosis; combine with `--once`). |

See the [quickstart](quickstart.md#validating-against-a-live-cluster) for the
live-cluster validation recipe combining `--once --debug --trace`.

## Full example

```yaml
server:
  host: "0.0.0.0"
  port: "9444"
  uri: "/metrics"
  logName: "/var/log/pscale_exporter/pscale-exporter.log"

collection:
  interval: "30s"   # OneFS perf samples are ~30s native; capacity changes slowly
  timeout: "20s"

opentelemetry:
  metrics:
    enabled: false
    endpoint: "localhost:4317"
    insecure: true
    interval: "30s"
  tracing:
    enabled: false
    endpoint: "localhost:4317"
    insecure: true
    samplingRate: 0.1

# A read-only account is sufficient. Grant its role read access to:
#   ISI_PRIV_STATISTICS  (CPU, capacity, cache, drive, client & protocol stats)
#   ISI_PRIV_QUOTA       (quota usage / thresholds)
#   ISI_PRIV_DEVICES     (node inventory, hardware, node health)
#   ISI_PRIV_EVENT       (active event groups)
#   ISI_PRIV_SNAPSHOT    (snapshot count & used space)
#   ISI_PRIV_SYNCIQ      (SyncIQ replication policy health)
#   ISI_PRIV_SMB         (SMB share count)
#   ISI_PRIV_NFS         (NFS export count)
#   ISI_PRIV_LICENSE     (license status & expiry)
#   ISI_PRIV_SMARTPOOLS  (storage-pool / tier capacity)
clusters:
  - name: pscale-cluster1
    endpoint: pscale-clu1.example.com
    port: 8080
    username: pscale-monitor
    password: "${PSCALE1_PASSWORD}"
    # Skip TLS certificate verification for this cluster (INSECURE — use only with a
    # self-signed cert on a trusted network). Accepts a literal bool or a ${VAR} reference:
    #   insecureSkipVerify: true
    #   insecureSkipVerify: "${PSCALE1_SKIP_CERTIFICATE}"
    insecureSkipVerify: false
```

## Blocks

### `server`

The HTTP listener. `uri` is the metrics path (`/metrics`); `/health` is always served and
is snapshot-based. `logName` is the log file path — make sure the runtime user can write
it (the container image pre-creates `/var/log/pscale_exporter` owned by `uid 10001`).

### `collection`

- `interval` — how often the background loop polls every cluster and publishes a new
  snapshot. OneFS performance samples are natively ~30s, so going below that buys little.
- `timeout` — per-collection deadline for a cluster.

Both export paths read the **latest snapshot** rather than fetching on scrape, so the
scrape/push cadence is independent of OneFS API load.

### `opentelemetry`

Optional OTLP push for metrics and/or tracing. Disabled by default. See
[OpenTelemetry](../opentelemetry.md) for the full push path.

### `clusters`

A list — one entry per OneFS cluster. Every metric carries a `cluster` label set to
`name`. `insecureSkipVerify` should stay `false` in production; enable it only for lab
clusters with self-signed certificates. It accepts either a literal bool
(`insecureSkipVerify: true`) or a `${VAR}` environment reference resolved at startup
(`insecureSkipVerify: "${PSCALE1_SKIP_CERTIFICATE}"`), the same pattern used for
`endpoint`/`username`/`password` — disabling TLS verification should still be used only
with self-signed certs on a trusted network.

## Secrets

Never put plaintext passwords in the file. Two options:

- **Environment reference** — `password: "${PSCALE1_PASSWORD}"` expands the named env var
  at load time. Export it before starting the process (or pass `-e` to `docker run`).
  The same expansion works for `endpoint` and `username` — all three fields support `${VAR}`.
- **`passwordFile`** — point at a file whose contents are the password (handy with
  Kubernetes/Docker secrets mounted as files).

### Passwords with special characters

Any character is safe end to end — the credentials are sent to the OneFS session-login
API by the `gopowerscale` SDK (encoded) over TLS, so nothing needs URL-encoding. The only
place quoting matters is **parsing at load time**, and it differs by where you put the
password:

| Source | Rule |
| --- | --- |
| `.env`, single-quoted `'…'` | Fully literal — no `$` expansion, no `\` escapes, no `#` comment. Best default. Cannot contain a literal `'`. |
| `.env`, double-quoted `"…"` | Expands `$VAR`/`${VAR}` and processes `\` escapes. `$`, `\`, `"` are special — write `\$`, `\\`, `\"`. |
| `.env`, unquoted | `$VAR` expands; a `#` (space-hash) starts a comment; a value **starting** with `'`/`"` is treated as quoted. |
| `config.yaml` inline | Only the exact `${NAME}` token is interpolated (`os.LookupEnv`), so a literal password containing `${NAME}` is treated as an env ref. Prefer referencing an env var. |
| `passwordFile` | Read **verbatim** (only surrounding whitespace trimmed) — no interpolation, no escaping. The bulletproof option. |

For quotes inside the password specifically: use double quotes to include a `'`, single
quotes to include a `"`. If the password has **both** `'` and `"` (or a `\`, or starts
with a quote), use `passwordFile` — it needs no escaping at all. When referencing an env
var from `config.yaml` (`password: "${PSCALE1_PASSWORD}"`) the value is inserted verbatim
and never re-scanned, so the env var itself may contain `$`, `${…}`, or any character.

## .env loading

`pscale_exporter` loads a `.env` file **natively at startup** — before config
interpolation — so the `cp .env.example .env` quickstart works for bare-metal runs as
well as Docker Compose (which reads `.env` natively).

Search order (first found wins):

1. `.env` in the **working directory** (typical for direct invocations).
2. `.env` next to **`config.yaml`** (covers systemd units whose `WorkingDirectory` is
   not the install dir).

**No-override semantics**: already-set environment variables always win. A stray `.env`
file can never shadow a secret injected via the shell, a secrets manager, or a container
environment. The load is silent when no `.env` file is found.

## Environment variables / .env

The compose files pass through a set of `PSCALE1_*` variables as a **single-cluster
quickstart convenience** — the `1` in the name is literal and scopes these to the first
(and only) cluster in the default `config.yaml`.

```
PSCALE1_HOSTNAME=onefs.example.com   # → config clusters[0].endpoint
PSCALE1_USERNAME=pscale-monitor      # → config clusters[0].username
PSCALE1_PASSWORD=secret              # → config clusters[0].password
```

`config.yaml` is always the source of truth and is always consumed by the exporter.
The compose `environment:` block just injects the shell (or `.env`) values into the
container so the `${PSCALE1_*}` references inside `config.yaml` resolve correctly.

Copy `.env.example` to `.env`, fill in real values, and run:

```bash
docker compose up -d --build
```

### Multi-cluster

For more than one cluster, add one entry per cluster in `config.yaml`.  You can use
literal values or introduce your own env refs — you just have to also pass them through in
`docker-compose.yml`'s `environment:` block.  The compose files do **not** auto-discover
`PSCALE2_*` etc.; you add them explicitly.

Example `config.yaml` snippet for two clusters:

```yaml
clusters:
  - name: pscale-cluster1
    endpoint: "${PSCALE1_HOSTNAME}"
    port: 8080
    username: "${PSCALE1_USERNAME}"
    password: "${PSCALE1_PASSWORD}"
    insecureSkipVerify: false

  - name: pscale-cluster2
    endpoint: "${PSCALE2_HOSTNAME}"
    port: 8080
    username: "${PSCALE2_USERNAME}"
    password: "${PSCALE2_PASSWORD}"
    insecureSkipVerify: false
```

And the matching addition to the `pscale_exporter` service's `environment:` in
`docker-compose.yml`:

```yaml
    environment:
      - PSCALE1_HOSTNAME=${PSCALE1_HOSTNAME:-pscale-clu1.example.com}
      - PSCALE1_USERNAME=${PSCALE1_USERNAME:-pscale-monitor}
      - PSCALE1_PASSWORD=${PSCALE1_PASSWORD:-}
      - PSCALE2_HOSTNAME=${PSCALE2_HOSTNAME:-}
      - PSCALE2_USERNAME=${PSCALE2_USERNAME:-}
      - PSCALE2_PASSWORD=${PSCALE2_PASSWORD:-}
```

## Hot reload

The exporter reloads its config without a restart on **`SIGHUP`** and on **file change**
(it watches the config file). Edit `config.yaml`, and the next collection cycle picks up
the new clusters/intervals.
