package powerscale

import (
	"strings"
	"testing"
	"time"

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

// TestStatKeyDivisors guards the unit contract of the curated table: a key whose row omits
// "divisor" must pass its raw value through, and every cpu.*.avg key must divide by ten
// because OneFS reports those in tenths of a percent (idle+user+sys sums to 1000).
func TestStatKeyDivisors(t *testing.T) {
	if got := statKeyByKey["ifs.bytes.total"].scale(5000); got != 5000 {
		t.Fatalf("omitted divisor must pass the value through, got %v", got)
	}
	if got := (StatKeySpec{}).scale(42); got != 42 {
		t.Fatalf("zero spec must not divide by zero, got %v", got)
	}
	cpuKeys := 0
	for _, s := range statKeySpecs {
		// A negative or absurd divisor would silently skew a metric.
		if s.Divisor < 0 {
			t.Errorf("%s: divisor = %v, must not be negative", s.Key, s.Divisor)
		}
		if !strings.HasPrefix(s.Key, "cluster.cpu.") && !strings.HasPrefix(s.Key, "node.cpu.") {
			continue
		}
		cpuKeys++
		if s.Divisor != 10 {
			t.Errorf("%s: divisor = %v, want 10 (OneFS reports tenths of a percent)", s.Key, s.Divisor)
		}
		if !strings.HasSuffix(s.Metric, "_percent") {
			t.Errorf("%s: metric %q should carry the _percent suffix", s.Key, s.Metric)
		}
	}
	if cpuKeys != 6 {
		t.Fatalf("expected 6 cpu keys in the curated table, found %d", cpuKeys)
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
			// OneFS reports cpu.*.avg in tenths of a percent, so these must be scaled.
			{Key: "cluster.cpu.sys.avg", DevID: 0, Value: 125},
			{Key: "node.cpu.idle.avg", DevID: 2, Value: 886},
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
	// Tenths of a percent must reach the _percent metric as 0-100, at both scopes, and
	// land exactly so the exposition output stays free of float artifacts.
	if s, ok := get("powerscale_cluster_cpu_sys_percent"); !ok || s.Value != 12.5 {
		t.Fatalf("cluster cpu sample not rescaled to percent: %+v ok=%v want 12.5", s, ok)
	}
	if s, ok := get("powerscale_node_cpu_idle_percent"); !ok || s.Value != 88.6 {
		t.Fatalf("node cpu sample not rescaled to percent: %+v ok=%v want 88.6", s, ok)
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
	// Nodes in this inventory carry no hardware block, so no info metric is invented.
	if s, ok := get("powerscale_node_hardware_info"); ok {
		t.Fatalf("hardware info emitted for a node with no hardware identity: %+v", s)
	}
}

// TestHardwareInfoSample covers the info metric that lets a dashboard explain empty
// fan/temperature/power-supply panels.
func TestHardwareInfoSample(t *testing.T) {
	// A node with no hardware block is skipped rather than emitting empty labels.
	if s, ok := hardwareInfoSample("clu1", "GUID-1", "2", models.Node{LNN: 2}); ok {
		t.Fatalf("hardware info emitted for a node with no hardware identity: %+v", s)
	}
	node := models.Node{LNN: 1, Product: "SIMULATOR-1U-Dual-6144MB-1x1GE-100GB",
		Series: "virtual_series", HWGen: "VMware"}
	s, ok := hardwareInfoSample("clu1", "GUID-1", "1", node)
	if !ok {
		t.Fatalf("no info sample for a node carrying a hardware block")
	}
	if s.Name != "powerscale_node_hardware_info" || s.Value != 1 {
		t.Fatalf("info sample wrong: %+v", s)
	}
	want := map[string]string{
		"cluster": "clu1", "cluster_id": "GUID-1", "node": "1",
		"product": "SIMULATOR-1U-Dual-6144MB-1x1GE-100GB",
		"series":  "virtual_series", "hwgen": "VMware",
	}
	for _, l := range s.Labels {
		if want[l.Name] != l.Value {
			t.Errorf("label %s = %q, want %q", l.Name, l.Value, want[l.Name])
		}
		delete(want, l.Name)
	}
	if len(want) != 0 {
		t.Errorf("missing labels: %v", want)
	}
}

// TestExplainMissingHardware covers the operator-facing explanation for absent hardware
// metrics: all-virtual, a physical node with a silent sensor subsystem, and the healthy
// case where nothing needs explaining. (The function lives in collector.go; the cases sit
// here beside the hardware sample derivations they explain.)
func TestExplainMissingHardware(t *testing.T) {
	virtual := models.Node{LNN: 1, Series: "virtual_series", HWGen: "VMware",
		Product: "SIMULATOR-1U-Dual-6144MB-1x1GE-100GB"}
	healthy := models.Node{LNN: 2, PowerSupplies: 2,
		Temperatures: []models.Sensor{{Name: "CPU0", Value: 35}}}

	msg := explainMissingHardware([]models.Node{virtual, {LNN: 2, Series: "virtual_series", HWGen: "VMware"}})
	if !strings.Contains(msg, "2/2 nodes report virtual hardware") ||
		!strings.Contains(msg, "series=virtual_series") ||
		!strings.Contains(msg, "powerscale_node_fan_speed_rpm") {
		t.Fatalf("all-virtual message unhelpful: %q", msg)
	}

	// A physical node reporting nothing is a different story and must not be called virtual.
	msg = explainMissingHardware([]models.Node{healthy, {LNN: 3, Series: "h_series", HWGen: "Gen6"}})
	if !strings.Contains(msg, "1/2 nodes report no power supplies") || !strings.Contains(msg, "nodes 3") {
		t.Fatalf("partial message unhelpful: %q", msg)
	}
	if strings.Contains(msg, "virtual") {
		t.Errorf("physical node described as virtual: %q", msg)
	}

	if msg := explainMissingHardware([]models.Node{healthy}); msg != "" {
		t.Errorf("nothing to explain, got %q", msg)
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
	syncExp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	inv := &models.Inventory{
		Cluster: models.ClusterInfo{Name: "ignored", GUID: "GUID-1"},
		Licenses: []models.License{
			{Name: "SyncIQ", Status: "Licensed", ExpirationUnix: syncExp},
			{Name: "SmartQuotas", Status: "Expired", ExpirationUnix: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()},
			{Name: "SnapshotIQ", Status: "Licensed", ExpirationUnix: 0},
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
	if s, ok := find("powerscale_license_expiration_timestamp_seconds", "SyncIQ"); !ok || s.Value != float64(syncExp) {
		t.Fatalf("SyncIQ expiration timestamp wrong: %+v ok=%v (want %d)", s, ok, syncExp)
	}
	if s, ok := find("powerscale_license_expiration_timestamp_seconds", "SnapshotIQ"); !ok || s.Value != 0 {
		t.Fatalf("perpetual license (SnapshotIQ) must emit expiration timestamp 0: %+v ok=%v", s, ok)
	}
	if s, ok := find("powerscale_license_active", "SmartQuotas"); !ok || s.Value != 0 {
		t.Fatalf("SmartQuotas (Expired) active should be 0: %+v ok=%v", s, ok)
	}
	s, ok := find("powerscale_license_active", "SyncIQ")
	if !ok || s.Value != 1 {
		t.Fatalf("SyncIQ (Licensed) active should be 1: %+v ok=%v", s, ok)
	}
	hasStatus := false
	for _, l := range s.Labels {
		if l.Name == "status" && l.Value == "Licensed" {
			hasStatus = true
		}
	}
	if !hasStatus {
		t.Fatalf("active sample missing status label: %+v", s.Labels)
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
