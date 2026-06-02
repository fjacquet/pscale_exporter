package models

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
