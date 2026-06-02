package powerscale

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
