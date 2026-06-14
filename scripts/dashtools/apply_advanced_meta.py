"""Apply descriptions, legend calcs, and thresholds to the Advanced dashboard."""
from lib import ADVANCED, load, dump, iter_panels

PROV = "Provisional: stat keys not yet live-validated against an OneFS cluster."

DESCRIPTIONS = {
    "Nodes Read-only": "Count of nodes mounted read-only (powerscale_node_readonly=1). Should be 0.",
    "Nodes Smartfailing": "Count of nodes smartfailing or smartfailed. Should be 0.",
    "Active Critical Events": "Unresolved OneFS event-group occurrences at critical severity.",
    "SyncIQ Policies Failed": "SyncIQ replication policies whose last run failed or needs attention.",
    "Drives by State": "Drive counts grouped by drive state (HEALTHY, SMARTFAIL, DEAD, ...).",
    "Active Events by Severity": "Unresolved OneFS event-group occurrences over time, by severity.",
    "SyncIQ Policies": "SyncIQ replication policies with enabled state and last-run result.",
    "Snapshot Space Used": "Aggregate space held by all snapshots.",
    "Cache Read Hit vs Miss (L1/L2/L3)": "L1/L2/L3 cache read hit vs miss throughput. " + PROV,
    "Cache Hit Ratio by Level": "Read-cache hit ratio per level. " + PROV,
    "Node CPU (sys / user / idle)": "Per-node CPU split into system, user, and idle.",
    "Quota Usage vs Thresholds": "Per-quota logical/physical usage against advisory, soft, and hard thresholds.",
    "Dedupe Logical Saved": "Logical space saved by deduplication. " + PROV,
    "Deduplicated Data": "Logical data that has been deduplicated. " + PROV,
    "Top Drive IOPS": "Highest per-drive operation rates. " + PROV,
    "Drive Busy %": "Per-drive busy time. " + PROV,
    "Client Operations by Protocol/Class": "Per-client operation rate by protocol and class. " + PROV,
    "Client Throughput (in / out)": "Per-client inbound/outbound throughput. " + PROV,
    "Power-Supply Failures": "Failed power supplies across nodes. Should be 0. " + PROV,
    "Node Temperature": "Node temperature sensor readings. " + PROV,
    "Fan Speed": "Node fan speed readings. " + PROV,
}

BOOL_BAD = {"mode": "absolute", "steps": [
    {"color": "green", "value": None},
    {"color": "red", "value": 1},
]}
THRESHOLDS = {
    "Nodes Read-only": BOOL_BAD,
    "Nodes Smartfailing": BOOL_BAD,
    "Active Critical Events": BOOL_BAD,
    "SyncIQ Policies Failed": BOOL_BAD,
    "Power-Supply Failures": BOOL_BAD,
}
LEGEND = {"displayMode": "table", "placement": "bottom",
          "calcs": ["lastNotNull", "max", "mean"]}
TOOLTIP = {"mode": "multi", "sort": "desc"}


def apply(dash):
    for p in iter_panels(dash):
        title = p.get("title")
        if title in DESCRIPTIONS:
            p["description"] = DESCRIPTIONS[title]
        fc = p.setdefault("fieldConfig", {}).setdefault("defaults", {})
        if title in THRESHOLDS:
            fc["thresholds"] = THRESHOLDS[title]
        if p.get("type") == "timeseries":
            opts = p.setdefault("options", {})
            opts["legend"] = LEGEND
            opts["tooltip"] = TOOLTIP


dash = load(ADVANCED)
apply(dash)
dump(ADVANCED, dash)
print("advanced metadata applied")
