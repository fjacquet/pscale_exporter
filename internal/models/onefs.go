package models

import (
	"encoding/json"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// flexFloat parses a JSON number OR a numeric string (OneFS sensor readings are sometimes
// quoted, e.g. "35.0"). Unparseable/empty values decode to 0 rather than erroring, keeping
// the surrounding best-effort parse resilient; the fallback is logged at debug so a
// schema surprise on a remote system leaves a trace.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Debugf("flexFloat: unparseable value %q decoded to 0", string(b))
		return nil
	}
	*f = flexFloat(v)
	return nil
}

// Sensor is one hardware sensor reading (temperature or fan).
type Sensor struct {
	Name  string
	Value float64
}

// sensorGroup is one sensor category in the nodes payload (e.g. "Temps", "Fans").
type sensorGroup struct {
	Name   string `json:"name"`
	Values []struct {
		Name  string    `json:"name"`
		Value flexFloat `json:"value"`
	} `json:"values"`
}

// sensorGroups accepts both shapes OneFS uses for a node's "sensors" field: live 9.x
// payloads wrap the list in an object ({"sensors": [...]}), older payloads use a bare
// array. Any other shape decodes to empty rather than failing the whole nodes parse —
// sensors are best-effort hardware telemetry — but is logged at debug so a schema
// surprise on a remote system leaves a trace instead of silently missing metrics.
type sensorGroups []sensorGroup

func (s *sensorGroups) UnmarshalJSON(b []byte) error {
	var wrapped struct {
		Sensors []sensorGroup `json:"sensors"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil {
		*s = wrapped.Sensors
		return nil
	}
	var flat []sensorGroup
	if err := json.Unmarshal(b, &flat); err == nil {
		*s = flat
		return nil
	}
	const max = 200
	trace := string(b)
	if len(trace) > max {
		trace = trace[:max] + "..."
	}
	log.Debugf("sensors: unrecognized shape decoded to empty: %s", trace)
	return nil
}

// ClusterInfo identifies a cluster (from platform/3/cluster/config).
type ClusterInfo struct {
	Name    string
	GUID    string
	Release string
}

// Node is one cluster node (from platform/3/cluster/nodes). Beyond identity it carries
// best-effort health derived from the node's state and drive list.
type Node struct {
	ID  int // device id (devid)
	LNN int // logical node number

	Readonly  bool // node mounted read-only (state.readonly.enabled)
	Smartfail bool // node is smartfailing / smartfailed (state.smartfail.state)

	// DrivesByState counts drives by their UI state (e.g. "HEALTHY", "SMARTFAIL",
	// "DEAD"). Empty when the nodes payload carries no drive list.
	DrivesByState map[string]int

	// Hardware (best-effort, from status.powersupplies + sensors; shape validated
	// against a live OneFS 9.13 virtual cluster).
	PowerSupplies       int // status.powersupplies.count
	PowerSupplyFailures int // status.powersupplies.failures
	Temperatures        []Sensor
	Fans                []Sensor
}

// Quota is one directory quota (from platform/1/quota/quotas). Threshold fields are 0
// when the corresponding threshold is unset (null).
type Quota struct {
	ID            string
	Path          string
	Type          string
	UsageBytes    float64 // logical usage (usage.fslogical)
	PhysicalBytes float64 // physical usage (usage.fsphysical)
	HardBytes     float64
	SoftBytes     float64
	AdvisoryBytes float64
}

// Counts holds simple inventory counts.
type Counts struct {
	NFSExports int
	SMBShares  int
	Snapshots  int
}

// SnapshotSummary is the aggregate snapshot space usage (snapshot/snapshots-summary).
type SnapshotSummary struct {
	UsedBytes float64
}

// SyncPolicy is one SyncIQ replication policy (sync/policies).
type SyncPolicy struct {
	Name         string
	Enabled      bool
	LastJobState string // e.g. "finished", "failed", "needs_attention", "running"
}

// DedupeSummary is cluster-wide deduplication/efficiency (dedupe/dedupe-summary).
// OneFS reports block counts; bytes are derived as blocks * block_size.
type DedupeSummary struct {
	LogicalSavedBytes float64 // saved_logical_blocks * block_size
	DeduplicatedBytes float64 // logical_blocks * block_size
}

// Inventory is the typed OneFS state for one cluster at one collection cycle. The trailing
// fields are best-effort: a fetch/parse failure leaves them zero-valued without failing
// the snapshot.
type Inventory struct {
	Cluster      ClusterInfo
	Nodes        []Node
	Quotas       []Quota
	Counts       Counts
	Snapshot     SnapshotSummary
	SyncPolicies []SyncPolicy
	Events       map[string]int // unresolved event-group count by severity
	Dedupe       DedupeSummary
}

// DriveStat is one per-drive performance row (statistics/summary/drive).
type DriveStat struct {
	Node        int     // LNN, parsed from drive_id "LNN:bay"
	Bay         string  // bay, parsed from drive_id "LNN:bay"
	Type        string  // e.g. "SSD", "HDD"
	OpsPerSec   float64 // xfers_in + xfers_out
	BusyPercent float64 // busy (0-100)
}

// ClientStat is one per-client-class row (statistics/summary/client). Aggregated by
// node/protocol/class to bound cardinality. PROVISIONAL schema — best-effort.
type ClientStat struct {
	Node      int
	Protocol  string
	Class     string
	OpsPerSec float64 // operation_rate
	InBps     float64 // in
	OutBps    float64 // out
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
	Drives  []DriveStat
	Clients []ClientStat
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

// ParseNodes parses platform/N/cluster/nodes, including best-effort node health from the
// node state and drive list when the payload carries them.
func ParseNodes(b []byte) ([]Node, error) {
	var raw struct {
		Nodes []struct {
			ID    int `json:"id"`
			LNN   int `json:"lnn"`
			State struct {
				Readonly struct {
					Enabled bool `json:"enabled"`
				} `json:"readonly"`
				// Smartfail uses boolean sub-fields only (9.14.0+). The legacy
				// state string is no longer present in 9.14 payloads.
				Smartfail struct {
					Smartfailed bool `json:"smartfailed"`
				} `json:"smartfail"`
			} `json:"state"`
			Drives []struct {
				UIState string `json:"ui_state"`
				// Present is a pointer so payloads without the field (older
				// schemas) keep counting every listed drive.
				Present *bool `json:"present"`
			} `json:"drives"`
			Status struct {
				Powersupplies struct {
					Count    int `json:"count"`
					Failures int `json:"failures"`
				} `json:"powersupplies"`
			} `json:"status"`
			Sensors sensorGroups `json:"sensors"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(raw.Nodes))
	for _, n := range raw.Nodes {
		var drives map[string]int
		for _, d := range n.Drives {
			if d.Present != nil && !*d.Present {
				continue // empty bay, not a drive
			}
			state := d.UIState
			if state == "" {
				state = "UNKNOWN"
			}
			if drives == nil {
				drives = make(map[string]int, len(n.Drives))
			}
			drives[state]++
		}
		var temps, fans []Sensor
		for _, grp := range n.Sensors {
			g := strings.ToLower(grp.Name)
			for _, v := range grp.Values {
				s := Sensor{Name: v.Name, Value: float64(v.Value)}
				switch {
				case strings.Contains(g, "temp"):
					temps = append(temps, s)
				case strings.Contains(g, "fan"):
					fans = append(fans, s)
				}
			}
		}
		nodes = append(nodes, Node{
			ID: n.ID, LNN: n.LNN,
			Readonly:            n.State.Readonly.Enabled,
			Smartfail:           n.State.Smartfail.Smartfailed,
			DrivesByState:       drives,
			PowerSupplies:       n.Status.Powersupplies.Count,
			PowerSupplyFailures: n.Status.Powersupplies.Failures,
			Temperatures:        temps,
			Fans:                fans,
		})
	}
	return nodes, nil
}

// ParseQuotas parses platform/N/quota/quotas. A null threshold yields 0 for that field.
func ParseQuotas(b []byte) ([]Quota, error) {
	var raw struct {
		Quotas []struct {
			ID    string `json:"id"`
			Path  string `json:"path"`
			Type  string `json:"type"`
			Usage struct {
				FSLogical  float64 `json:"fslogical"`
				FSPhysical float64 `json:"fsphysical"`
			} `json:"usage"`
			Thresholds struct {
				Hard     *float64 `json:"hard"`
				Soft     *float64 `json:"soft"`
				Advisory *float64 `json:"advisory"`
			} `json:"thresholds"`
		} `json:"quotas"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	deref := func(p *float64) float64 {
		if p != nil {
			return *p
		}
		return 0
	}
	quotas := make([]Quota, 0, len(raw.Quotas))
	for _, q := range raw.Quotas {
		quotas = append(quotas, Quota{
			ID: q.ID, Path: q.Path, Type: q.Type,
			UsageBytes:    q.Usage.FSLogical,
			PhysicalBytes: q.Usage.FSPhysical,
			HardBytes:     deref(q.Thresholds.Hard),
			SoftBytes:     deref(q.Thresholds.Soft),
			AdvisoryBytes: deref(q.Thresholds.Advisory),
		})
	}
	return quotas, nil
}

// ParseSnapshotSummary parses platform/N/snapshot/snapshots-summary. UsedBytes prefers the
// active size (space held by live snapshots) and falls back to the total size.
func ParseSnapshotSummary(b []byte) (SnapshotSummary, error) {
	var raw struct {
		Summary struct {
			ActiveSize *float64 `json:"active_size"`
			Size       *float64 `json:"size"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return SnapshotSummary{}, err
	}
	// Prefer active_size; fall back to size only when active_size is absent (not when
	// it is a legitimate zero).
	var used float64
	switch {
	case raw.Summary.ActiveSize != nil:
		used = *raw.Summary.ActiveSize
	case raw.Summary.Size != nil:
		used = *raw.Summary.Size
	}
	return SnapshotSummary{UsedBytes: used}, nil
}

// ParseSyncPolicies parses platform/N/sync/policies.
func ParseSyncPolicies(b []byte) ([]SyncPolicy, error) {
	var raw struct {
		Policies []struct {
			Name         string `json:"name"`
			Enabled      bool   `json:"enabled"`
			LastJobState string `json:"last_job_state"`
		} `json:"policies"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]SyncPolicy, 0, len(raw.Policies))
	for _, p := range raw.Policies {
		out = append(out, SyncPolicy{Name: p.Name, Enabled: p.Enabled, LastJobState: p.LastJobState})
	}
	return out, nil
}

// ParseEventOccurrences parses platform/N/event/eventgroup-occurrences and returns a count
// of unresolved occurrences keyed by severity (e.g. "critical", "warning", "information").
func ParseEventOccurrences(b []byte) (map[string]int, error) {
	var raw struct {
		Eventgroups []struct {
			Severity string `json:"severity"`
			Resolved bool   `json:"resolved"`
		} `json:"eventgroups"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, e := range raw.Eventgroups {
		if e.Resolved {
			continue
		}
		sev := e.Severity
		if sev == "" {
			sev = "unknown"
		}
		counts[sev]++
	}
	return counts, nil
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

// ParseDedupeSummary parses platform/N/dedupe/dedupe-summary. The OneFS schema reports
// block counts, not bytes; bytes are derived as blocks * block_size.
func ParseDedupeSummary(b []byte) (DedupeSummary, error) {
	var raw struct {
		Summary struct {
			SavedLogicalBlocks *float64 `json:"saved_logical_blocks"`
			LogicalBlocks      *float64 `json:"logical_blocks"`
			BlockSize          *float64 `json:"block_size"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return DedupeSummary{}, err
	}
	deref := func(p *float64) float64 {
		if p != nil {
			return *p
		}
		return 0
	}
	bs := deref(raw.Summary.BlockSize)
	return DedupeSummary{
		LogicalSavedBytes: deref(raw.Summary.SavedLogicalBlocks) * bs,
		DeduplicatedBytes: deref(raw.Summary.LogicalBlocks) * bs,
	}, nil
}

// ParseDriveSummary parses platform/N/statistics/summary/drive. The OneFS schema returns
// a "drive" array whose items carry drive_id ("LNN:bay") and per-direction transfer rates;
// ops/sec is the sum of read+write transfer rates.
func ParseDriveSummary(b []byte) ([]DriveStat, error) {
	var raw struct {
		Drive []struct {
			DriveID  string  `json:"drive_id"`
			Type     string  `json:"type"`
			Busy     float64 `json:"busy"`
			XfersIn  float64 `json:"xfers_in"`
			XfersOut float64 `json:"xfers_out"`
		} `json:"drive"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]DriveStat, 0, len(raw.Drive))
	for _, d := range raw.Drive {
		lnn, bay, ok := splitDriveID(d.DriveID)
		if !ok {
			log.Debugf("drive summary: unparseable drive_id %q skipped", d.DriveID)
			continue
		}
		out = append(out, DriveStat{
			Node: lnn, Bay: bay, Type: d.Type,
			OpsPerSec:   d.XfersIn + d.XfersOut,
			BusyPercent: d.Busy,
		})
	}
	return out, nil
}

// splitDriveID parses an OneFS drive_id "LNN:bay" into its node LNN and bay string.
func splitDriveID(s string) (lnn int, bay string, ok bool) {
	i := strings.IndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return 0, "", false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, "", false
	}
	// bay is everything after the first colon; "1:2:3" yields bay="2:3".
	return n, s[i+1:], true
}

// ParseClientSummary parses platform/N/statistics/summary/client. PROVISIONAL schema.
func ParseClientSummary(b []byte) ([]ClientStat, error) {
	var raw struct {
		Client []struct {
			Node     int     `json:"node"`
			Protocol string  `json:"protocol"`
			Class         string  `json:"class"`
			OperationRate float64 `json:"operation_rate"`
			In            float64 `json:"in"`
			Out           float64 `json:"out"`
		} `json:"client"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]ClientStat, 0, len(raw.Client))
	for _, c := range raw.Client {
		out = append(out, ClientStat{
			Node: c.Node, Protocol: c.Protocol, Class: c.Class,
			OpsPerSec: c.OperationRate, InBps: c.In, OutBps: c.Out,
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
