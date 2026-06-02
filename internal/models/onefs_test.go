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
