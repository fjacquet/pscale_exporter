package models

import "encoding/json"

// ClusterInfo identifies a cluster (from platform/3/cluster/config).
type ClusterInfo struct {
	Name    string
	GUID    string
	Release string
}

// Node is one cluster node (from platform/3/cluster/nodes).
type Node struct {
	ID  int // device id (devid)
	LNN int // logical node number

	Status string
}

// Quota is one directory quota (from platform/1/quota/quotas).
type Quota struct {
	ID         string
	Path       string
	Type       string
	UsageBytes float64
	HardBytes  float64 // 0 if no hard threshold
}

// Counts holds simple inventory counts.
type Counts struct {
	NFSExports int
	SMBShares  int
	Snapshots  int
}

// Inventory is the typed OneFS state for one cluster at one collection cycle.
type Inventory struct {
	Cluster ClusterInfo
	Nodes   []Node
	Quotas  []Quota
	Counts  Counts
}

// StatPoint is one resolved statistics value. DevID 0 means the cluster aggregate;
// >0 maps to a node LNN.
type StatPoint struct {
	Key   string
	DevID int
	Value float64
}

// ProtoStat is one protocol-summary row.
type ProtoStat struct {
	Node          int
	Protocol      string
	Operation     string
	OperationRate float64 // ops/sec
	LatencyAvg    float64 // microseconds
}

// Statistics holds the raw statistics fetched for one cluster.
type Statistics struct {
	Current []StatPoint
	Proto   []ProtoStat
}

// ParseClusterConfig parses platform/N/cluster/config.
func ParseClusterConfig(b []byte) (ClusterInfo, error) {
	var raw struct {
		Name         string `json:"name"`
		GUID         string `json:"guid"`
		OneFSVersion struct {
			Release string `json:"release"`
		} `json:"onefs_version"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return ClusterInfo{}, err
	}
	return ClusterInfo{Name: raw.Name, GUID: raw.GUID, Release: raw.OneFSVersion.Release}, nil
}

// ParseNodes parses platform/N/cluster/nodes.
func ParseNodes(b []byte) ([]Node, error) {
	var raw struct {
		Nodes []struct {
			ID     int    `json:"id"`
			LNN    int    `json:"lnn"`
			Status string `json:"status"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(raw.Nodes))
	for _, n := range raw.Nodes {
		nodes = append(nodes, Node{ID: n.ID, LNN: n.LNN, Status: n.Status})
	}
	return nodes, nil
}

// ParseQuotas parses platform/N/quota/quotas. A null hard threshold yields HardBytes 0.
func ParseQuotas(b []byte) ([]Quota, error) {
	var raw struct {
		Quotas []struct {
			ID    string `json:"id"`
			Path  string `json:"path"`
			Type  string `json:"type"`
			Usage struct {
				FSLogical float64 `json:"fslogical"`
			} `json:"usage"`
			Thresholds struct {
				Hard *float64 `json:"hard"`
			} `json:"thresholds"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	quotas := make([]Quota, 0, len(raw.Quotas))
	for _, q := range raw.Quotas {
		var hard float64
		if q.Thresholds.Hard != nil {
			hard = *q.Thresholds.Hard
		}
		quotas = append(quotas, Quota{
			ID: q.ID, Path: q.Path, Type: q.Type,
			UsageBytes: q.Usage.FSLogical, HardBytes: hard,
		})
	}
	return quotas, nil
}

// ParseStatCurrent parses platform/N/statistics/current. Rows with a non-null error or
// a non-scalar value are skipped.
func ParseStatCurrent(b []byte) ([]StatPoint, error) {
	var raw struct {
		Stats []struct {
			DevID int             `json:"devid"`
			Error *string         `json:"error"`
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	pts := make([]StatPoint, 0, len(raw.Stats))
	for _, s := range raw.Stats {
		if s.Error != nil {
			continue
		}
		var v float64
		if err := json.Unmarshal(s.Value, &v); err != nil {
			continue // skip non-scalar values
		}
		pts = append(pts, StatPoint{Key: s.Key, DevID: s.DevID, Value: v})
	}
	return pts, nil
}

// ParseProtocolSummary parses platform/N/statistics/summary/protocol.
func ParseProtocolSummary(b []byte) ([]ProtoStat, error) {
	var raw struct {
		Protocol []struct {
			Node          int     `json:"node"`
			Protocol      string  `json:"protocol"`
			Operation     string  `json:"operation"`
			OperationRate float64 `json:"operation_rate"`
			TimeAvg       float64 `json:"time_avg"`
		} `json:"protocol"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]ProtoStat, 0, len(raw.Protocol))
	for _, p := range raw.Protocol {
		out = append(out, ProtoStat{
			Node: p.Node, Protocol: p.Protocol, Operation: p.Operation,
			OperationRate: p.OperationRate, LatencyAvg: p.TimeAvg,
		})
	}
	return out, nil
}

// ParseTotal extracts the "total" field used by list endpoints (exports, shares,
// snapshots) for cheap inventory counts.
func ParseTotal(b []byte) (int, error) {
	var raw struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return 0, err
	}
	return raw.Total, nil
}
