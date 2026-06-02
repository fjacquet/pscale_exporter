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
	return samples
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
		if q.HardBytes > 0 {
			out = append(out, Sample{Name: "powerscale_quota_hard_threshold_bytes", Labels: labels, Value: q.HardBytes})
		}
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
