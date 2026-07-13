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

// driveLabels appends node identity and a drive UI-state in canonical order.
func driveLabels(clusterName, clusterID, nodeLNN, state string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "node", Value: nodeLNN},
		Label{Name: "state", Value: state},
	)
}

// policyLabels appends a SyncIQ policy name.
func policyLabels(clusterName, clusterID, policy string) []Label {
	return append(baseLabels(clusterName, clusterID), Label{Name: "policy", Value: policy})
}

// severityLabels appends an event severity.
func severityLabels(clusterName, clusterID, severity string) []Label {
	return append(baseLabels(clusterName, clusterID), Label{Name: "severity", Value: severity})
}

// licenseLabels appends a licensed-feature name.
func licenseLabels(clusterName, clusterID, name string) []Label {
	return append(baseLabels(clusterName, clusterID), Label{Name: "name", Value: name})
}

// licenseActiveLabels appends a licensed-feature name and its OneFS status string,
// used on the powerscale_license_active gauge.
func licenseActiveLabels(clusterName, clusterID, name, status string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "name", Value: name},
		Label{Name: "status", Value: status},
	)
}

// driveStatLabels builds the canonical per-drive label set.
func driveStatLabels(clusterName, clusterID, nodeLNN, bay, driveType string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "node", Value: nodeLNN},
		Label{Name: "bay", Value: bay},
		Label{Name: "type", Value: driveType},
	)
}

// clientLabels builds the canonical per-client-class label set.
func clientLabels(clusterName, clusterID, nodeLNN, proto, class string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "node", Value: nodeLNN},
		Label{Name: "protocol", Value: proto},
		Label{Name: "class", Value: class},
	)
}

// sensorLabels appends node identity and a hardware sensor name.
func sensorLabels(clusterName, clusterID, nodeLNN, sensor string) []Label {
	return append(baseLabels(clusterName, clusterID),
		Label{Name: "node", Value: nodeLNN},
		Label{Name: "sensor", Value: sensor},
	)
}
