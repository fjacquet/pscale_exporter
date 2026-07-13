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
		Nodes: []models.Node{
			{ID: 1, LNN: 1, DrivesByState: map[string]int{"HEALTHY": 2},
				PowerSupplies: 2, PowerSupplyFailures: 0,
				Temperatures: []models.Sensor{{Name: "CPU0", Value: 35}},
				Fans:         []models.Sensor{{Name: "Fan1", Value: 4500}}},
			{ID: 2, LNN: 2, Readonly: true, DrivesByState: map[string]int{"HEALTHY": 1, "SMARTFAIL": 1},
				PowerSupplies: 2, PowerSupplyFailures: 1},
		},
		Quotas: []models.Quota{
			{ID: "q1", Path: "/ifs/data/a", Type: "directory", UsageBytes: 100, HardBytes: 1000, SoftBytes: 800, AdvisoryBytes: 600, PhysicalBytes: 120},
		},
		Counts:       models.Counts{NFSExports: 5, SMBShares: 3, Snapshots: 7},
		Snapshot:     models.SnapshotSummary{UsedBytes: 10240},
		SyncPolicies: []models.SyncPolicy{{Name: "dr", Enabled: true, LastJobState: "failed"}},
		Events:       map[string]int{"critical": 2},
		Dedupe:       models.DedupeSummary{LogicalSavedBytes: 1000, DeduplicatedBytes: 5000},
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
		Drives: []models.DriveStat{
			{Node: 1, Bay: "1", Type: "SSD", OpsPerSec: 120, BusyPercent: 15.5},
		},
		Clients: []models.ClientStat{
			{Node: 1, Protocol: "nfs3", Class: "read", OpsPerSec: 50, InBps: 1024, OutBps: 2048},
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

	if s, ok := get("powerscale_quota_soft_threshold_bytes"); !ok || s.Value != 800 {
		t.Fatalf("quota soft sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_quota_advisory_threshold_bytes"); !ok || s.Value != 600 {
		t.Fatalf("quota advisory sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_quota_physical_usage_bytes"); !ok || s.Value != 120 {
		t.Fatalf("quota physical sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_node_readonly"); !ok {
		t.Fatalf("node readonly sample missing: %+v", s)
	}
	if s, ok := get("powerscale_node_drives_total"); !ok || s.Value == 0 {
		t.Fatalf("node drives sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_snapshot_used_bytes"); !ok || s.Value != 10240 {
		t.Fatalf("snapshot used sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_synciq_last_run_failed"); !ok || s.Value != 1 {
		t.Fatalf("synciq failed sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_synciq_policy_enabled"); !ok || s.Value != 1 {
		t.Fatalf("synciq enabled sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_active_events"); !ok || s.Value != 2 {
		t.Fatalf("active events sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_dedupe_logical_saved_bytes"); !ok || s.Value != 1000 {
		t.Fatalf("dedupe saved sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_drive_operations_per_second"); !ok || s.Value != 120 {
		t.Fatalf("drive ops sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_drive_busy_percent"); !ok || s.Value != 15.5 {
		t.Fatalf("drive busy sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_client_operations_per_second"); !ok || s.Value != 50 {
		t.Fatalf("client ops sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_client_in_bytes_per_second"); !ok || s.Value != 1024 {
		t.Fatalf("client in sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_node_power_supplies_total"); !ok || s.Value != 2 {
		t.Fatalf("psu total sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_node_temperature_celsius"); !ok || s.Value != 35 {
		t.Fatalf("temperature sample wrong: %+v ok=%v", s, ok)
	}
	if s, ok := get("powerscale_node_fan_speed_rpm"); !ok || s.Value != 4500 {
		t.Fatalf("fan sample wrong: %+v ok=%v", s, ok)
	}
}

func TestBuildSamplesNodeIfsCacheKeys(t *testing.T) {
	inv := &models.Inventory{
		Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
		Nodes:   []models.Node{{ID: 1, LNN: 1}},
	}
	st := &models.Statistics{
		Current: []models.StatPoint{
			{Key: "node.ifs.cache.l1.data.read.hit", DevID: 1, Value: 1000},
			{Key: "node.ifs.cache.l1.data.read.miss", DevID: 1, Value: 100},
			{Key: "node.ifs.cache.l2.data.read.hit", DevID: 1, Value: 2000},
			{Key: "node.ifs.cache.l2.data.read.miss", DevID: 1, Value: 200},
			{Key: "node.ifs.cache.l3.data.read.hit", DevID: 1, Value: 3000},
			{Key: "node.ifs.cache.l3.data.read.miss", DevID: 1, Value: 300},
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
	cases := []struct {
		metric string
		value  float64
	}{
		{"powerscale_node_cache_l1_read_hit_bytes_per_second", 1000},
		{"powerscale_node_cache_l1_read_miss_bytes_per_second", 100},
		{"powerscale_node_cache_l2_read_hit_bytes_per_second", 2000},
		{"powerscale_node_cache_l2_read_miss_bytes_per_second", 200},
		{"powerscale_node_cache_l3_read_hit_bytes_per_second", 3000},
		{"powerscale_node_cache_l3_read_miss_bytes_per_second", 300},
	}
	for _, c := range cases {
		s, ok := get(c.metric)
		if !ok || s.Value != c.value {
			t.Fatalf("cache sample %s wrong: %+v ok=%v", c.metric, s, ok)
		}
		if s.Labels[2].Value != "1" { // nodeLabels = [cluster, cluster_id, node]
			t.Fatalf("cache sample %s node label wrong: %+v", c.metric, s.Labels)
		}
	}
}

func TestBuildSamplesLicenses(t *testing.T) {
	inv := &models.Inventory{
		Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
		Licenses: []models.License{
			{Name: "SyncIQ", Status: "Licensed", DaysToExpiry: 214, HasExpiration: true, Expired: false},
			{Name: "SmartQuotas", Status: "Expired", DaysToExpiry: 0, HasExpiration: true, Expired: true},
			{Name: "SnapshotIQ", Status: "Licensed", DaysToExpiry: 0, HasExpiration: false, Expired: false},
		},
	}
	samples := BuildSamples("clu1", inv, nil)
	find := func(name, feature string) (Sample, bool) {
		for _, s := range samples {
			if s.Name != name {
				continue
			}
			for _, l := range s.Labels {
				if l.Name == "name" && l.Value == feature {
					return s, true
				}
			}
		}
		return Sample{}, false
	}
	if s, ok := find("powerscale_license_days_to_expiry", "SyncIQ"); !ok || s.Value != 214 {
		t.Fatalf("SyncIQ days_to_expiry wrong: %+v ok=%v", s, ok)
	}
	if _, ok := find("powerscale_license_days_to_expiry", "SnapshotIQ"); ok {
		t.Fatal("perpetual license (SnapshotIQ) must not emit days_to_expiry")
	}
	if s, ok := find("powerscale_license_expired", "SmartQuotas"); !ok || s.Value != 1 {
		t.Fatalf("SmartQuotas expired should be 1: %+v ok=%v", s, ok)
	}
	s, ok := find("powerscale_license_info", "SyncIQ")
	if !ok || s.Value != 1 {
		t.Fatalf("SyncIQ info missing: %+v ok=%v", s, ok)
	}
	hasStatus := false
	for _, l := range s.Labels {
		if l.Name == "status" && l.Value == "Licensed" {
			hasStatus = true
		}
	}
	if !hasStatus {
		t.Fatalf("info sample missing status label: %+v", s.Labels)
	}
}

func TestBuildSamplesStoragePools(t *testing.T) {
	inv := &models.Inventory{
		Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
		StoragePools: []models.StoragePool{
			{Name: "nodepool1", Type: "nodepool", TotalBytes: 3000, UsedBytes: 1000, AvailBytes: 2000,
				SSDTotalBytes: 1000, SSDUsedBytes: 400, SSDAvailBytes: 600,
				HDDTotalBytes: 2000, HDDUsedBytes: 600, HDDAvailBytes: 1400},
			{Name: "hdd_pool", Type: "nodepool", TotalBytes: 2000, UsedBytes: 600, AvailBytes: 1400,
				SSDTotalBytes: 0, SSDUsedBytes: 0, SSDAvailBytes: 0,
				HDDTotalBytes: 2000, HDDUsedBytes: 600, HDDAvailBytes: 1400},
		},
	}
	samples := BuildSamples("clu1", inv, nil)
	find := func(name, pool string) (Sample, bool) {
		for _, s := range samples {
			if s.Name != name {
				continue
			}
			for _, l := range s.Labels {
				if l.Name == "pool" && l.Value == pool {
					return s, true
				}
			}
		}
		return Sample{}, false
	}
	if s, ok := find("powerscale_storagepool_total_capacity_bytes", "nodepool1"); !ok || s.Value != 3000 {
		t.Fatalf("nodepool1 total wrong: %+v ok=%v", s, ok)
	}
	if s, ok := find("powerscale_storagepool_ssd_used_capacity_bytes", "nodepool1"); !ok || s.Value != 400 {
		t.Fatalf("nodepool1 ssd_used wrong: %+v ok=%v", s, ok)
	}
	// the all-HDD pool still emits an ssd_total series, valued 0 (always-emit)
	if s, ok := find("powerscale_storagepool_ssd_total_capacity_bytes", "hdd_pool"); !ok || s.Value != 0 {
		t.Fatalf("hdd_pool ssd_total should be present and 0: %+v ok=%v", s, ok)
	}
	// the type label is present
	s, ok := find("powerscale_storagepool_total_capacity_bytes", "nodepool1")
	hasType := false
	for _, l := range s.Labels {
		if l.Name == "type" && l.Value == "nodepool" {
			hasType = true
		}
	}
	if !ok || !hasType {
		t.Fatalf("nodepool1 missing type label: %+v", s.Labels)
	}
}

func TestBuildSamplesWorkloads(t *testing.T) {
	inv := &models.Inventory{Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"}}
	st := &models.Statistics{
		Workloads: []models.Workload{
			{Node: 1, Zone: "System", Protocol: "nfs3", Username: "alice", Ops: 120, BytesIn: 1024, BytesOut: 2048, CPUMicros: 50000},
			{Node: 0, Ops: 5, BytesIn: 10, BytesOut: 20, CPUMicros: 100}, // aggregate: all dims empty
		},
	}
	samples := BuildSamples("clu1", inv, st)
	find := func(name, username string) (Sample, bool) {
		for _, s := range samples {
			if s.Name != name {
				continue
			}
			for _, l := range s.Labels {
				if l.Name == "username" && l.Value == username {
					return s, true
				}
			}
		}
		return Sample{}, false
	}
	if s, ok := find("powerscale_workload_operations_per_second", "alice"); !ok || s.Value != 120 {
		t.Fatalf("alice ops wrong: %+v ok=%v", s, ok)
	}
	if s, ok := find("powerscale_workload_cpu_microseconds_per_second", "alice"); !ok || s.Value != 50000 {
		t.Fatalf("alice cpu wrong: %+v ok=%v", s, ok)
	}
	// aggregate row: username="" and zone="" but still emits values (empty-label path)
	s, ok := find("powerscale_workload_in_bytes_per_second", "")
	if !ok || s.Value != 10 {
		t.Fatalf("aggregate in_bytes wrong: %+v ok=%v", s, ok)
	}
	hasEmptyZone := false
	for _, l := range s.Labels {
		if l.Name == "zone" && l.Value == "" {
			hasEmptyZone = true
		}
	}
	if !hasEmptyZone {
		t.Fatalf("aggregate row should carry an empty zone label: %+v", s.Labels)
	}
}
