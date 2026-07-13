package powerscale

import (
	"strconv"
	"strings"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

// BuildSamples converts one cluster's inventory and statistics into exported samples.
func BuildSamples(clusterName string, inv *models.Inventory, st *models.Statistics) []Sample {
	if inv == nil {
		return nil
	}
	clusterID := inv.Cluster.GUID

	// node LNN lookup by devid for node-scoped stat keys.
	lnnByDevID := make(map[int]int, len(inv.Nodes))
	for _, n := range inv.Nodes {
		lnnByDevID[n.ID] = n.LNN
	}

	var samples []Sample
	samples = append(samples, statSamples(clusterName, clusterID, st, lnnByDevID)...)
	samples = append(samples, quotaSamples(clusterName, clusterID, inv.Quotas)...)
	samples = append(samples, countSamples(clusterName, clusterID, inv.Counts)...)
	samples = append(samples, protoSamples(clusterName, clusterID, st)...)
	samples = append(samples, nodeHealthSamples(clusterName, clusterID, inv.Nodes)...)
	samples = append(samples, hardwareSamples(clusterName, clusterID, inv.Nodes)...)
	samples = append(samples, snapshotSamples(clusterName, clusterID, inv.Snapshot)...)
	samples = append(samples, syncSamples(clusterName, clusterID, inv.SyncPolicies)...)
	samples = append(samples, eventSamples(clusterName, clusterID, inv.Events)...)
	samples = append(samples, dedupeSamples(clusterName, clusterID, inv.Dedupe)...)
	samples = append(samples, licenseSamples(clusterName, clusterID, inv.Licenses)...)
	samples = append(samples, storagePoolSamples(clusterName, clusterID, inv.StoragePools)...)
	samples = append(samples, driveSamples(clusterName, clusterID, st)...)
	samples = append(samples, clientSamples(clusterName, clusterID, st)...)
	samples = append(samples, workloadSamples(clusterName, clusterID, st)...)
	return samples
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func statSamples(clusterName, clusterID string, st *models.Statistics, lnnByDevID map[int]int) []Sample {
	if st == nil {
		return nil
	}
	var out []Sample
	for _, p := range st.Current {
		spec, ok := statKeyByKey[p.Key]
		if !ok {
			continue
		}
		switch spec.Scope {
		case "node":
			lnn, ok := lnnByDevID[p.DevID]
			if !ok {
				continue
			}
			out = append(out, Sample{
				Name:   spec.Metric,
				Labels: nodeLabels(clusterName, clusterID, strconv.Itoa(lnn)),
				Value:  p.Value,
			})
		default: // "cluster"
			out = append(out, Sample{
				Name:   spec.Metric,
				Labels: baseLabels(clusterName, clusterID),
				Value:  p.Value,
			})
		}
	}
	return out
}

func quotaSamples(clusterName, clusterID string, quotas []models.Quota) []Sample {
	var out []Sample
	for _, q := range quotas {
		labels := quotaLabels(clusterName, clusterID, q.ID, q.Path, q.Type)
		out = append(out, Sample{Name: "powerscale_quota_usage_bytes", Labels: labels, Value: q.UsageBytes})
		if q.PhysicalBytes > 0 {
			out = append(out, Sample{Name: "powerscale_quota_physical_usage_bytes", Labels: labels, Value: q.PhysicalBytes})
		}
		if q.HardBytes > 0 {
			out = append(out, Sample{Name: "powerscale_quota_hard_threshold_bytes", Labels: labels, Value: q.HardBytes})
		}
		if q.SoftBytes > 0 {
			out = append(out, Sample{Name: "powerscale_quota_soft_threshold_bytes", Labels: labels, Value: q.SoftBytes})
		}
		if q.AdvisoryBytes > 0 {
			out = append(out, Sample{Name: "powerscale_quota_advisory_threshold_bytes", Labels: labels, Value: q.AdvisoryBytes})
		}
	}
	return out
}

// nodeHealthSamples emits per-node read-only / smartfail state and drive counts by UI
// state. The drive-state series let a single PromQL query alert on any non-HEALTHY drive.
func nodeHealthSamples(clusterName, clusterID string, nodes []models.Node) []Sample {
	var out []Sample
	for _, n := range nodes {
		lnn := strconv.Itoa(n.LNN)
		base := nodeLabels(clusterName, clusterID, lnn)
		out = append(out,
			Sample{Name: "powerscale_node_readonly", Labels: base, Value: b2f(n.Readonly)},
			Sample{Name: "powerscale_node_smartfail", Labels: base, Value: b2f(n.Smartfail)},
		)
		for state, count := range n.DrivesByState {
			out = append(out, Sample{
				Name:   "powerscale_node_drives_total",
				Labels: driveLabels(clusterName, clusterID, lnn, state),
				Value:  float64(count),
			})
		}
	}
	return out
}

// hardwareSamples emits per-node power-supply health and temperature/fan sensor readings.
// PSU samples are emitted only when the node reports supplies; sensors only when present.
func hardwareSamples(clusterName, clusterID string, nodes []models.Node) []Sample {
	var out []Sample
	for _, n := range nodes {
		lnn := strconv.Itoa(n.LNN)
		if n.PowerSupplies > 0 {
			base := nodeLabels(clusterName, clusterID, lnn)
			out = append(out,
				Sample{Name: "powerscale_node_power_supplies_total", Labels: base, Value: float64(n.PowerSupplies)},
				Sample{Name: "powerscale_node_power_supply_failures", Labels: base, Value: float64(n.PowerSupplyFailures)},
			)
		}
		for _, t := range n.Temperatures {
			out = append(out, Sample{
				Name:   "powerscale_node_temperature_celsius",
				Labels: sensorLabels(clusterName, clusterID, lnn, t.Name),
				Value:  t.Value,
			})
		}
		for _, f := range n.Fans {
			out = append(out, Sample{
				Name:   "powerscale_node_fan_speed_rpm",
				Labels: sensorLabels(clusterName, clusterID, lnn, f.Name),
				Value:  f.Value,
			})
		}
	}
	return out
}

// snapshotSamples emits aggregate snapshot space usage. The gauge is always emitted —
// including a real 0 — so it is distinguishable from missing data, matching countSamples.
func snapshotSamples(clusterName, clusterID string, s models.SnapshotSummary) []Sample {
	return []Sample{{
		Name:   "powerscale_snapshot_used_bytes",
		Labels: baseLabels(clusterName, clusterID),
		Value:  s.UsedBytes,
	}}
}

// syncFailureStates are SyncIQ last-job states treated as a failed/attention-needed run.
var syncFailureStates = map[string]bool{"failed": true, "needs_attention": true, "unknown": true}

// syncSamples emits per-policy SyncIQ replication health.
func syncSamples(clusterName, clusterID string, policies []models.SyncPolicy) []Sample {
	var out []Sample
	for _, p := range policies {
		labels := policyLabels(clusterName, clusterID, p.Name)
		out = append(out,
			Sample{Name: "powerscale_synciq_policy_enabled", Labels: labels, Value: b2f(p.Enabled)},
			Sample{Name: "powerscale_synciq_last_run_failed", Labels: labels, Value: b2f(syncFailureStates[p.LastJobState])},
		)
	}
	return out
}

// licenseSamples emits per-feature license state, aligned with issue #34: an absolute
// expiration timestamp (0 for perpetual/unlicensed features, which carry no expiration) and
// an active gauge (1 when the feature is currently licensed/usable) that carries the raw
// OneFS status as a label. Both are emitted for every license.
func licenseSamples(clusterName, clusterID string, licenses []models.License) []Sample {
	var out []Sample
	for _, l := range licenses {
		out = append(out,
			Sample{Name: "powerscale_license_expiration_timestamp_seconds", Labels: licenseLabels(clusterName, clusterID, l.Name), Value: float64(l.ExpirationUnix)},
			Sample{Name: "powerscale_license_active", Labels: licenseActiveLabels(clusterName, clusterID, l.Name, l.Status), Value: b2f(licenseActive(l.Status))},
		)
	}
	return out
}

// storagePoolSamples emits per-pool/per-tier capacity: the aggregate plus an SSD/HDD media
// split. The list contains both node pools and tiers (a tier's capacity is the sum of its
// child node pools), distinguished by the type label — summing across all rows double-counts,
// so consumers filter type="nodepool" for a non-overlapping cluster total. All 9 gauges are
// always emitted (an all-HDD pool simply reports ssd=0).
func storagePoolSamples(clusterName, clusterID string, pools []models.StoragePool) []Sample {
	var out []Sample
	for _, p := range pools {
		labels := storagePoolLabels(clusterName, clusterID, p.Name, p.Type)
		out = append(out,
			Sample{Name: "powerscale_storagepool_total_capacity_bytes", Labels: labels, Value: p.TotalBytes},
			Sample{Name: "powerscale_storagepool_used_capacity_bytes", Labels: labels, Value: p.UsedBytes},
			Sample{Name: "powerscale_storagepool_available_capacity_bytes", Labels: labels, Value: p.AvailBytes},
			Sample{Name: "powerscale_storagepool_ssd_total_capacity_bytes", Labels: labels, Value: p.SSDTotalBytes},
			Sample{Name: "powerscale_storagepool_ssd_used_capacity_bytes", Labels: labels, Value: p.SSDUsedBytes},
			Sample{Name: "powerscale_storagepool_ssd_available_capacity_bytes", Labels: labels, Value: p.SSDAvailBytes},
			Sample{Name: "powerscale_storagepool_hdd_total_capacity_bytes", Labels: labels, Value: p.HDDTotalBytes},
			Sample{Name: "powerscale_storagepool_hdd_used_capacity_bytes", Labels: labels, Value: p.HDDUsedBytes},
			Sample{Name: "powerscale_storagepool_hdd_available_capacity_bytes", Labels: labels, Value: p.HDDAvailBytes},
		)
	}
	return out
}

// licenseActive reports whether a OneFS license status means the feature is currently
// usable. OneFS reports Licensed/Activated for a paid license and Evaluation during a trial;
// any other status (Expired, Unlicensed, Unknown) means the feature is not active.
func licenseActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "licensed", "activated", "evaluation":
		return true
	default:
		return false
	}
}

// eventSamples emits the count of unresolved event groups per severity.
func eventSamples(clusterName, clusterID string, events map[string]int) []Sample {
	var out []Sample
	for severity, count := range events {
		out = append(out, Sample{
			Name:   "powerscale_active_events",
			Labels: severityLabels(clusterName, clusterID, severity),
			Value:  float64(count),
		})
	}
	return out
}

// dedupeSamples emits cluster-wide deduplication efficiency. Always emitted (best-effort
// failure yields 0) so the gauges are distinguishable from missing data.
func dedupeSamples(clusterName, clusterID string, d models.DedupeSummary) []Sample {
	base := baseLabels(clusterName, clusterID)
	return []Sample{
		{Name: "powerscale_dedupe_logical_saved_bytes", Labels: base, Value: d.LogicalSavedBytes},
		{Name: "powerscale_dedupe_deduplicated_bytes", Labels: base, Value: d.DeduplicatedBytes},
	}
}

// driveSamples emits per-drive performance.
func driveSamples(clusterName, clusterID string, st *models.Statistics) []Sample {
	if st == nil {
		return nil
	}
	var out []Sample
	for _, d := range st.Drives {
		labels := driveStatLabels(clusterName, clusterID, strconv.Itoa(d.Node), d.Bay, d.Type)
		out = append(out,
			Sample{Name: "powerscale_drive_operations_per_second", Labels: labels, Value: d.OpsPerSec},
			Sample{Name: "powerscale_drive_busy_percent", Labels: labels, Value: d.BusyPercent},
		)
	}
	return out
}

// clientSamples emits per-client-class operation rate and throughput.
func clientSamples(clusterName, clusterID string, st *models.Statistics) []Sample {
	if st == nil {
		return nil
	}
	var out []Sample
	for _, c := range st.Clients {
		labels := clientLabels(clusterName, clusterID, strconv.Itoa(c.Node), c.Protocol, c.Class)
		out = append(out,
			Sample{Name: "powerscale_client_operations_per_second", Labels: labels, Value: c.OpsPerSec},
			Sample{Name: "powerscale_client_in_bytes_per_second", Labels: labels, Value: c.InBps},
			Sample{Name: "powerscale_client_out_bytes_per_second", Labels: labels, Value: c.OutBps},
		)
	}
	return out
}

// workloadSamples emits per-workload performance (ops, throughput, CPU). Rows come from OneFS
// performance datasets; the identity dimensions are labels (unpinned ones are ""). All four
// gauges are per-second rates — aggregate with sum/avg, never rate().
func workloadSamples(clusterName, clusterID string, st *models.Statistics) []Sample {
	if st == nil {
		return nil
	}
	var out []Sample
	for _, w := range st.Workloads {
		labels := workloadLabels(clusterName, clusterID, strconv.Itoa(w.Node), w.Zone, w.Protocol, w.Username, w.SystemName, w.JobType)
		out = append(out,
			Sample{Name: "powerscale_workload_operations_per_second", Labels: labels, Value: w.Ops},
			Sample{Name: "powerscale_workload_in_bytes_per_second", Labels: labels, Value: w.BytesIn},
			Sample{Name: "powerscale_workload_out_bytes_per_second", Labels: labels, Value: w.BytesOut},
			Sample{Name: "powerscale_workload_cpu_microseconds_per_second", Labels: labels, Value: w.CPUMicros},
		)
	}
	return out
}

func countSamples(clusterName, clusterID string, c models.Counts) []Sample {
	base := baseLabels(clusterName, clusterID)
	return []Sample{
		{Name: "powerscale_nfs_exports_total", Labels: base, Value: float64(c.NFSExports)},
		{Name: "powerscale_smb_shares_total", Labels: base, Value: float64(c.SMBShares)},
		{Name: "powerscale_snapshots_total", Labels: base, Value: float64(c.Snapshots)},
	}
}

func protoSamples(clusterName, clusterID string, st *models.Statistics) []Sample {
	if st == nil {
		return nil
	}
	var out []Sample
	for _, p := range st.Proto {
		labels := protocolLabels(clusterName, clusterID, strconv.Itoa(p.Node), p.Protocol, p.Operation)
		out = append(out,
			Sample{Name: "powerscale_protocol_operations_per_second", Labels: labels, Value: p.OperationRate},
			Sample{Name: "powerscale_protocol_latency_microseconds", Labels: labels, Value: p.LatencyAvg},
		)
	}
	return out
}
