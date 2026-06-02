package powerscale

import (
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

func TestStatKeysLoaded(t *testing.T) {
	if len(statKeySpecs) == 0 {
		t.Fatal("statKeySpecs not loaded from embedded JSON")
	}
	if statKeyByKey["ifs.bytes.total"].Metric != "powerscale_cluster_total_capacity_bytes" {
		t.Fatalf("mapping wrong: %+v", statKeyByKey["ifs.bytes.total"])
	}
}

func TestQueryKeyList(t *testing.T) {
	keys := QueryKeys()
	found := false
	for _, k := range keys {
		if k == "cluster.cpu.sys.avg" {
			found = true
		}
	}
	if !found {
		t.Fatal("QueryKeys missing cluster.cpu.sys.avg")
	}
}

func TestBuildSamplesClusterAndNode(t *testing.T) {
	inv := &models.Inventory{
		Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
		Nodes:   []models.Node{{ID: 1, LNN: 1}, {ID: 2, LNN: 2}},
		Quotas: []models.Quota{
			{ID: "q1", Path: "/ifs/data/a", Type: "directory", UsageBytes: 100, HardBytes: 1000},
		},
		Counts: models.Counts{NFSExports: 5, SMBShares: 3, Snapshots: 7},
	}
	st := &models.Statistics{
		Current: []models.StatPoint{
			{Key: "ifs.bytes.total", DevID: 0, Value: 5000},
			{Key: "node.memory.used", DevID: 2, Value: 42},
			{Key: "unmapped.key", DevID: 0, Value: 1}, // ignored
		},
		Proto: []models.ProtoStat{
			{Node: 1, Protocol: "nfs3", Operation: "read", OperationRate: 12, LatencyAvg: 800},
		},
	}

	samples := BuildSamples("clu1", inv, st)

	get := func(name string) (Sample, bool) {
		for _, s := range samples {
			if s.Name == name {
				return s, true
			}
		}
		return Sample{}, false
	}

	if s, ok := get("powerscale_cluster_total_capacity_bytes"); !ok || s.Value != 5000 {
		t.Fatalf("cluster capacity sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_node_memory_used_bytes"); !ok || s.Value != 42 || s.Labels[2].Value != "2" {
		t.Fatalf("node memory sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_quota_usage_bytes"); !ok || s.Value != 100 {
		t.Fatalf("quota usage sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_quota_hard_threshold_bytes"); !ok || s.Value != 1000 {
		t.Fatalf("quota hard sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_nfs_exports_total"); !ok || s.Value != 5 {
		t.Fatalf("nfs count sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_protocol_operations_per_second"); !ok || s.Value != 12 {
		t.Fatalf("protocol rate sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_protocol_latency_microseconds"); !ok || s.Value != 800 {
		t.Fatalf("protocol latency sample wrong: %+v ok=%v", s, ok)
	}
	if _, ok := get("powerscale_cluster_unmapped_key"); ok {
		t.Fatal("unmapped key should not produce a sample")
	}
}
