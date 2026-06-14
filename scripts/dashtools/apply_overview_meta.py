"""Apply descriptions, legend calcs, and thresholds to the Overview dashboard."""
from lib import OVERVIEW, load, dump, iter_panels

DESCRIPTIONS = {
    "Clusters Up": "Number of clusters scraping successfully (powerscale_up=1). Should equal your configured cluster count.",
    "Cluster Status": "Per-cluster scrape state; red means the last collection failed.",
    "OneFS API Version": "Detected OneFS platform API version negotiated at session start.",
    "Last Scrape Age": "Seconds since the last successful collection. Red indicates stale data (>2x the collection interval).",
    "NFS Exports": "Number of configured NFS exports.",
    "SMB Shares": "Number of configured SMB shares.",
    "Snapshots": "Total number of snapshots on the cluster.",
    "Capacity Used %": "Used /ifs capacity as a percent of total. Warning at 85%, critical at 95%.",
    "Cluster Capacity (used / available / total)": "Used, available, and total /ifs capacity over time.",
    "Top Quotas (usage vs hard limit)": "Quotas ranked by logical usage against their hard limit.",
    "Cluster CPU": "Cluster-aggregate CPU split into system, user, and idle.",
    "External Network Throughput": "External (front-end) inbound and outbound network throughput.",
    "Cluster Disk IOPS": "Cluster-wide disk transfer rate (ops/s). Aggregate with sum/avg, never rate().",
    "Protocol Operations": "Per-protocol operation rate (ops/s) by operation class.",
    "Protocol Latency": "Average per-protocol operation latency.",
    "Node CPU Idle": "Per-node idle CPU; low idle indicates CPU saturation.",
    "Node Memory Used": "Per-node memory in use.",
    "Node Disk IOPS": "Per-node disk transfer rate (ops/s).",
    "Node Used Capacity": "Per-node used /ifs capacity.",
}

# Panels that get directional thresholds (single-direction meaning only).
PERCENT_USED = {"steps": [
    {"color": "green", "value": None},
    {"color": "yellow", "value": 85},
    {"color": "red", "value": 95},
]}
STALENESS = {"steps": [
    {"color": "green", "value": None},
    {"color": "red", "value": 120},
]}
BOOL_BAD = {"steps": [  # 0 good, >0 bad
    {"color": "green", "value": None},
    {"color": "red", "value": 1},
]}
UP_GOOD = {"steps": [  # 0 bad, >=1 good
    {"color": "red", "value": None},
    {"color": "green", "value": 1},
]}

THRESHOLDS = {
    "Capacity Used %": PERCENT_USED,
    "Last Scrape Age": STALENESS,
    "Clusters Up": UP_GOOD,
    "Cluster Status": UP_GOOD,
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
            fc["thresholds"] = {"mode": "absolute", **THRESHOLDS[title]}
        if p.get("type") == "timeseries":
            opts = p.setdefault("options", {})
            opts["legend"] = LEGEND
            opts["tooltip"] = TOOLTIP


dash = load(OVERVIEW)
apply(dash)
dump(OVERVIEW, dash)
print("overview metadata applied")
