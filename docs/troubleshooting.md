# Troubleshooting

The exporter is built to be diagnosed **remotely**: when it runs on a site you cannot
reach, its debug output and response dumps carry everything needed to reproduce a
problem offline.

## Debug logging (`--debug`)

`--debug` raises the log level and emits, per collection cycle and per cluster:

- **Every API request** with the full URL, parameter count, response size, and duration:

  ```text
  cluster "pscale1": GET https://pscale-clu1.example.com:8080/platform/3/cluster/nodes (0 params): 14306 bytes in 42ms
  ```

- **A parse summary for the inventory** — node, quota, sensor-value, export/share/
  snapshot, SyncIQ-policy counts and unresolved events — and **for the statistics** —
  how many curated stat keys came back versus requested, naming the missing ones:

  ```text
  cluster "pscale1": inventory parsed: release=9.13.0.0 nodes=4 (sensor values=37) quotas=12 nfs_exports=3 smb_shares=5 snapshots=42 sync_policies=1 events=map[warning:2]
  cluster "pscale1": statistics parsed: keys=21/23 requested (missing: [ifs.bytes.deleted cluster.dedupe.estimated.saved.bytes]) proto_rows=18 drive_rows=16 client_rows=6
  ```

- **A payload snippet whenever a response fails to parse** (both required and
  best-effort endpoints), so the log shows what the API actually returned.

- **A trace whenever a lenient decoder falls back**: sensor payloads with an
  unrecognized shape and unparseable sensor values normally decode to empty/zero
  silently; at debug level each fallback is logged with the offending fragment.

A best-effort endpoint failing (dedupe, SyncIQ, drive/client summaries, …) is logged at
debug and leaves its metrics at zero — it never takes the cluster down.

## Response dumping (`--dump-dir`)

For a schema surprise that debug logs alone can't settle, ask the on-site operator to
run **one collection cycle with response dumping**:

```bash
pscale_exporter --config config.yaml --debug --once --dump-dir /tmp/pscale-dump
```

Every raw OneFS response body is written verbatim to
`/tmp/pscale-dump/<cluster>/<endpoint>.json`, e.g.:

```text
/tmp/pscale-dump/pscale1/platform_3_cluster_nodes.json
/tmp/pscale-dump/pscale1/platform_1_statistics_current.json
/tmp/pscale-dump/pscale1/platform_2_statistics_summary_protocol.json
```

The operator zips the directory and sends it back. The files are drop-in
`testdata/` fixtures, so the exact live payload becomes a unit test.

**Safety:** response bodies carry no credentials — OneFS session cookies live in HTTP
headers, never in bodies. Quota paths and share names do appear; treat the dump like
any internal inventory listing. Files are written `0600` in `0750` directories.

Without `--once` the exporter keeps running and overwrites each file every collection
interval — useful to capture a payload that only appears under load, but remember to
turn it off afterwards.

## Health endpoint

`/health` returns `200 OK` while at least one cluster scrapes successfully and
`503 UNHEALTHY` when all clusters are unreachable; before the first cycle completes it
reports `200 OK (starting)`. A cluster failing collection sets `powerscale_up{cluster=...}`
to 0 — alert on that rather than on `/health` for per-cluster visibility.
