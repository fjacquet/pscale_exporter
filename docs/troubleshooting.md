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

## API response tracing (`--trace`)

`--trace` logs **every OneFS API exchange** at info level: method, URL, status, and the
full response body. Use it when a metric you expect is absent — the exporter never
guesses values, so an unexpected payload shape shows up as a silently-missing sample,
and the trace shows what the cluster actually returned. Combined with `--once --debug`
it is the live-cluster validation recipe:

```bash
pscale_exporter --config config.yaml --once --debug --trace > validate.out
grep -v '^{' validate.out > samples.txt    # sorted exposition-style sample dump
grep 'API trace' validate.out              # raw OneFS payloads
```

**Token safety:** only response *bodies* are logged, never headers — OneFS session
credentials (the `isisessid` cookie and CSRF token) live exclusively in headers, and
the `/session/1/session` login exchange happens inside the gopowerscale SDK without
passing through the traced code path, so it can never appear in the trace. (For the
same reason the exact status code is shown only for failed requests; successful
responses are reported as `2xx`.)

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

## Empty fan / temperature / power-supply panels

Almost always a **virtual cluster**, not a broken exporter. A hypervisor exposes no physical
sensors, so OneFS reports `powersupplies.count: 0` and empty sensor groups, and the four
`powerscale_node_{temperature_celsius,fan_speed_rpm,power_supplies_total,power_supply_failures}`
metrics have nothing to emit. The exporter says so once per cluster at startup:

```console
$ ./bin/pscale_exporter --config config.yaml --once | grep 'virtual hardware'
cluster "pscale-cluster1": 4/4 nodes report virtual hardware (series=virtual_series,
hwgen=VMware, product=SIMULATOR-1U-Dual-6144MB-1x1GE-100GB) and expose no physical
sensors, so ... are absent for those nodes
```

Confirm it from the metrics themselves — a virtual node carries `series="virtual_series"`:

```console
$ curl -s localhost:9444/metrics | grep node_hardware_info
powerscale_node_hardware_info{cluster="pscale-cluster1",node="1",
  product="SIMULATOR-1U-Dual-6144MB-1x1GE-100GB",series="virtual_series",hwgen="VMware"} 1
```

Or straight from the API, which is also where to look if the log line says only *some* nodes
went quiet — on a physical cluster that is a real sensor fault worth chasing:

```console
$ ./bin/pscale_exporter --config config.yaml --once --dump-dir /tmp/dump
$ jq '.nodes[0] | {series: .hardware.series, psu: .status.powersupplies.count,
    sensors: [.sensors.sensors[] | select(.count > 0) | .name]}' \
    /tmp/dump/<cluster>/platform_3_cluster_nodes.json
```

The same reasoning covers the other metrics that go missing on an idle or unlicensed
cluster: `powerscale_synciq_*` needs at least one SyncIQ policy, the
`powerscale_quota_*_threshold_bytes` series need a quota with that threshold actually set,
`powerscale_license_expiration_timestamp_seconds` is 0 for licenses with no expiry, and
`powerscale_protocol_*` / `powerscale_client_*` only appear while OneFS is reporting recent
protocol activity — on an idle cluster they come and go between collection cycles.

## Health endpoint

`/health` always answers `200 OK` with a JSON body reporting every configured cluster's
cached status from the last collection cycle: `{"clusters": [{"cluster", "ok",
"last_scrape", "err"}]}`. It never returns a non-200 status, even when every cluster is
unreachable — a cluster being down is data the exporter reports, not a failure of the
exporter process. `/livez` and `/readyz` are separate, always-200 probe endpoints with no
dependency on cluster state; use those for `livenessProbe`/`readinessProbe` wiring. A
cluster failing collection sets `powerscale_up{cluster=...}` to 0 — alert on that or on
`/health`'s JSON body, never on any probe's HTTP status.
