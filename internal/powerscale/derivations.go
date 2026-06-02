package powerscale

import (
	"strconv"

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
	samples = append(samples, snapshotSamples(clusterName, clusterID, inv.Snapshot)...)
	samples = append(samples, syncSamples(clusterName, clusterID, inv.SyncPolicies)...)
	samples = append(samples, eventSamples(clusterName, clusterID, inv.Events)...)
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
