package models

import (
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "powerscale", "testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func TestParseClusterConfig(t *testing.T) {
	ci, err := ParseClusterConfig(read(t, "cluster_config.json"))
	if err != nil || ci.GUID != "000abc000def" || ci.Name != "clu1" {
		t.Fatalf("cluster config parse: %+v err=%v", ci, err)
	}
}

func TestParseNodes(t *testing.T) {
	nodes, err := ParseNodes(read(t, "nodes.json"))
	if err != nil || len(nodes) != 2 || nodes[1].LNN != 2 {
		t.Fatalf("nodes parse: %+v err=%v", nodes, err)
	}
	if nodes[0].Readonly || !nodes[1].Readonly {
		t.Fatalf("node readonly: n0=%v n1=%v", nodes[0].Readonly, nodes[1].Readonly)
	}
	if nodes[0].Smartfail || !nodes[1].Smartfail {
		t.Fatalf("node smartfail: n0=%v n1=%v (want false, true)", nodes[0].Smartfail, nodes[1].Smartfail)
	}
	if nodes[0].DrivesByState["HEALTHY"] != 2 || nodes[1].DrivesByState["SMARTFAIL"] != 1 {
		t.Fatalf("drive states: n0=%v n1=%v", nodes[0].DrivesByState, nodes[1].DrivesByState)
	}
	if nodes[0].PowerSupplies != 2 || nodes[0].PowerSupplyFailures != 0 || nodes[1].PowerSupplyFailures != 1 {
		t.Fatalf("psu: n0=%d/%d n1 failures=%d", nodes[0].PowerSupplies, nodes[0].PowerSupplyFailures, nodes[1].PowerSupplyFailures)
	}
	if len(nodes[0].Temperatures) != 1 || nodes[0].Temperatures[0].Value != 35 || nodes[0].Temperatures[0].Name != "CPU0" {
		t.Fatalf("temperatures (string value via flexFloat): %+v", nodes[0].Temperatures)
	}
	if len(nodes[0].Fans) != 1 || nodes[0].Fans[0].Value != 4500 {
		t.Fatalf("fans (numeric value): %+v", nodes[0].Fans)
	}
}

// TestParseNodesOneFS913 covers the live OneFS 9.13 nodes payload shape (validated
// against a virtual PowerScale): "sensors" is an object wrapping a nested "sensors"
// array, "state.smartfail" is a set of booleans (not a state string), and the drive
// list includes empty bays flagged "present": false.
func TestParseNodesOneFS913(t *testing.T) {
	payload := []byte(`{"nodes": [
	  {"id": 1, "lnn": 1,
	   "state": {"readonly": {"enabled": false},
	             "smartfail": {"dead": false, "down": false, "in_cluster": true, "smartfailed": true}},
	   "drives": [
	     {"baynum": 1, "present": true, "ui_state": "HEALTHY"},
	     {"baynum": 2, "present": true, "ui_state": "HEALTHY"},
	     {"baynum": 3, "present": false, "ui_state": "EMPTY"}
	   ],
	   "status": {"powersupplies": {"count": 0, "failures": 0, "status": "Power Supplies OK", "supplies": []}},
	   "sensors": {"sensors": [
	     {"count": 1, "name": "Temps", "values": [{"name": "CPU0", "units": "C", "value": "41.0"}]},
	     {"count": 0, "name": "Fans", "values": []}
	   ]}}
	], "total": 1}`)
	nodes, err := ParseNodes(payload)
	if err != nil {
		t.Fatalf("parse live-shaped nodes payload: %v", err)
	}
	if len(nodes) != 1 || nodes[0].LNN != 1 {
		t.Fatalf("nodes: %+v", nodes)
	}
	if !nodes[0].Smartfail {
		t.Fatalf("smartfail boolean shape not detected: %+v", nodes[0])
	}
	if nodes[0].DrivesByState["HEALTHY"] != 2 {
		t.Fatalf("present drive count: %+v", nodes[0].DrivesByState)
	}
	if _, ok := nodes[0].DrivesByState["EMPTY"]; ok {
		t.Fatalf("empty bay (present=false) counted as a drive: %+v", nodes[0].DrivesByState)
	}
	if len(nodes[0].Temperatures) != 1 || nodes[0].Temperatures[0].Value != 41 {
		t.Fatalf("nested sensors object not parsed: %+v", nodes[0].Temperatures)
	}
}

func TestParseQuotas(t *testing.T) {
	qs, err := ParseQuotas(read(t, "quotas.json"))
	if err != nil || len(qs) != 1 {
		t.Fatalf("quota parse err: %+v %v", qs, err)
	}
	if qs[0].UsageBytes != 100 || qs[0].HardBytes != 1000 || qs[0].Path != "/ifs/data/proj" {
		t.Fatalf("quota fields: %+v", qs[0])
	}
	if qs[0].PhysicalBytes != 120 || qs[0].SoftBytes != 800 || qs[0].AdvisoryBytes != 600 {
		t.Fatalf("quota threshold fields: %+v", qs[0])
	}
}

func TestParseSnapshotSummary(t *testing.T) {
	s, err := ParseSnapshotSummary(read(t, "snapshots_summary.json"))
	if err != nil || s.UsedBytes != 10240 {
		t.Fatalf("snapshot summary: %+v err=%v", s, err)
	}
}

func TestParseSyncPolicies(t *testing.T) {
	ps, err := ParseSyncPolicies(read(t, "sync_policies.json"))
	if err != nil || len(ps) != 2 {
		t.Fatalf("sync policies parse: %+v err=%v", ps, err)
	}
	if !ps[0].Enabled || ps[0].Name != "daily-dr" {
		t.Fatalf("policy[0]: %+v", ps[0])
	}
	if ps[1].Enabled || ps[1].LastJobState != "failed" {
		t.Fatalf("policy[1]: %+v", ps[1])
	}
}

func TestParseEventOccurrences(t *testing.T) {
	ev, err := ParseEventOccurrences(read(t, "events.json"))
	if err != nil {
		t.Fatalf("event parse err: %v", err)
	}
	if ev["critical"] != 1 || ev["warning"] != 1 {
		t.Fatalf("event counts (resolved should be excluded): %+v", ev)
	}
}

func TestParseStatCurrent(t *testing.T) {
	pts, err := ParseStatCurrent(read(t, "stat_current.json"))
	if err != nil || len(pts) != 4 {
		t.Fatalf("stat parse: %d err=%v", len(pts), err)
	}
	if pts[0].Key != "ifs.bytes.total" || pts[0].Value != 5000 {
		t.Fatalf("stat point[0]: %+v", pts[0])
	}
}

func TestParseProtocolSummary(t *testing.T) {
	ps, err := ParseProtocolSummary(read(t, "stat_protocol.json"))
	if err != nil || len(ps) != 1 || ps[0].OperationRate != 12 || ps[0].LatencyAvg != 800 {
		t.Fatalf("protocol parse: %+v err=%v", ps, err)
	}
}

func TestParseCount(t *testing.T) {
	if n, err := ParseTotal(read(t, "nfs_exports.json")); err != nil || n != 1 {
		t.Fatalf("count parse: %d err=%v", n, err)
	}
}

func TestParseDedupeSummary(t *testing.T) {
	d, err := ParseDedupeSummary(read(t, "dedupe_summary.json"))
	if err != nil {
		t.Fatalf("dedupe parse err: %v", err)
	}
	// bytes = blocks * block_size: saved 1000*8192, deduplicated 5000*8192
	if d.LogicalSavedBytes != 8192000 || d.DeduplicatedBytes != 40960000 {
		t.Fatalf("dedupe parse: %+v", d)
	}

	// Missing/zero block_size must yield 0, not panic (best-effort contract).
	d2, err2 := ParseDedupeSummary([]byte(`{"summary":{}}`))
	if err2 != nil || d2.LogicalSavedBytes != 0 || d2.DeduplicatedBytes != 0 {
		t.Fatalf("zero block_size: %+v err=%v", d2, err2)
	}
}

func TestParseDriveSummary(t *testing.T) {
	ds, err := ParseDriveSummary(read(t, "stat_drive.json"))
	if err != nil || len(ds) != 2 {
		t.Fatalf("drive parse: %+v err=%v", ds, err)
	}
	if ds[0].Node != 1 || ds[0].Bay != "1" || ds[0].Type != "SSD" || ds[0].OpsPerSec != 120 || ds[0].BusyPercent != 15.5 {
		t.Fatalf("drive[0] fields: %+v", ds[0])
	}
}

func TestSplitDriveID(t *testing.T) {
	cases := []struct {
		in      string
		wantLNN int
		wantBay string
		wantOK  bool
	}{
		{"1:1", 1, "1", true},
		{"12:bay4", 12, "bay4", true},
		{"1:2:3", 1, "2:3", true}, // bay is everything after the first colon
		{"", 0, "", false},
		{"abc", 0, "", false},
		{":bay", 0, "", false},
		{"1:", 0, "", false},
		{"abc:1", 0, "", false},
	}
	for _, c := range cases {
		lnn, bay, ok := splitDriveID(c.in)
		if lnn != c.wantLNN || bay != c.wantBay || ok != c.wantOK {
			t.Errorf("splitDriveID(%q) = (%d, %q, %v), want (%d, %q, %v)",
				c.in, lnn, bay, ok, c.wantLNN, c.wantBay, c.wantOK)
		}
	}
}

func TestParseClientSummary(t *testing.T) {
	cs, err := ParseClientSummary(read(t, "stat_client.json"))
	if err != nil || len(cs) != 2 {
		t.Fatalf("client parse: %+v err=%v", cs, err)
	}
	if cs[0].Protocol != "nfs3" || cs[0].Class != "read" || cs[0].OpsPerSec != 50 || cs[0].InBps != 1024 || cs[0].OutBps != 2048 {
		t.Fatalf("client[0] fields: %+v", cs[0])
	}
}

// TestParseNodesLegacySensors covers the older OneFS shape where "sensors" is a bare
// array (not wrapped in an object). The dual-shape support in sensorGroups must keep
// parsing it even though 9.14.0 fixtures use the wrapped shape.
func TestParseNodesLegacySensors(t *testing.T) {
	payload := []byte(`{"nodes":[{"id":1,"lnn":1,
	  "state":{"readonly":{"enabled":false},"smartfail":{"smartfailed":false}},
	  "sensors":[{"name":"Temps","values":[{"name":"CPU0","value":"40.0"}]}]}]}`)
	nodes, err := ParseNodes(payload)
	if err != nil {
		t.Fatalf("parse legacy nodes: %v", err)
	}
	if len(nodes) != 1 || len(nodes[0].Temperatures) != 1 || nodes[0].Temperatures[0].Value != 40 {
		t.Fatalf("flat sensors array not parsed: %+v", nodes)
	}
	if nodes[0].Smartfail {
		t.Fatalf("smartfailed:false must yield Smartfail=false: %+v", nodes[0])
	}
}
