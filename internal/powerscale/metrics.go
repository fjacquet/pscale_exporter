package powerscale

import "strings"

// Label is a single metric label name-value pair.
type Label struct {
	Name  string
	Value string
}

// Sample is one exported metric data point: a fully-qualified name, ordered labels
// (the first is always "cluster"), and a value. Feeds both export paths.
type Sample struct {
	Name   string
	Labels []Label
	Value  float64
}

// OneFS object type identifiers.
const (
	ObjCluster     = "cluster"
	ObjNode        = "node"
	ObjQuota       = "quota"
	ObjNFS         = "nfs"
	ObjSMB         = "smb"
	ObjSnapshot    = "snapshot"
	ObjStoragePool = "storagepool"
	ObjProtocol    = "protocol"
)

// metricPrefix maps an object type to its metric name prefix.
var metricPrefix = map[string]string{
	ObjCluster:     "powerscale_cluster",
	ObjNode:        "powerscale_node",
	ObjQuota:       "powerscale_quota",
	ObjNFS:         "powerscale_nfs",
	ObjSMB:         "powerscale_smb",
	ObjSnapshot:    "powerscale_snapshot",
	ObjStoragePool: "powerscale_storagepool",
	ObjProtocol:    "powerscale_protocol",
}

// baseLabels returns the cluster identity labels every metric carries.
func baseLabels(clusterName, clusterID string) []Label {
	return []Label{
		{Name: "cluster", Value: clusterName},
		{Name: "cluster_id", Value: clusterID},
	}
}

// nodeLabels appends the node identity in canonical order.
func nodeLabels(clusterName, clusterID, nodeLNN string) []Label {
	return append(baseLabels(clusterName, clusterID), Label{Name: "node", Value: nodeLNN})
}

// quotaLabels builds the canonical Quota label set.
func quotaLabels(clusterName, clusterID, quotaID, path, quotaType string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "quota_id", Value: quotaID},
		Label{Name: "quota_path", Value: path},
		Label{Name: "quota_type", Value: quotaType},
	)
}

// protocolLabels builds the canonical Protocol-summary label set.
func protocolLabels(clusterName, clusterID, nodeLNN, proto, op string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "node", Value: nodeLNN},
		Label{Name: "protocol", Value: proto},
		Label{Name: "op", Value: op},
	)
}

// toSnake converts a dotted/camelCase OneFS stat key to snake_case
// ("cluster.cpu.sys.avg" -> "cluster_cpu_sys_avg").
func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '.' || r == '-':
			b.WriteByte('_')
		case r >= 'A' && r <= 'Z':
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
