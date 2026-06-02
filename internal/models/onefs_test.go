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
}

func TestParseQuotas(t *testing.T) {
	qs, err := ParseQuotas(read(t, "quotas.json"))
	if err != nil || len(qs) != 1 {
		t.Fatalf("quota parse err: %+v %v", qs, err)
	}
	if qs[0].UsageBytes != 100 || qs[0].HardBytes != 1000 || qs[0].Path != "/ifs/data/proj" {
		t.Fatalf("quota fields: %+v", qs[0])
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
