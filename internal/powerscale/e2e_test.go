package powerscale

import (
	"context"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
	"github.com/prometheus/client_golang/prometheus"
)

func TestEndToEndCollectionThroughPrometheus(t *testing.T) {
	srv := newMockOneFS(t)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := models.ClusterConfig{
		Name: "clu1", Endpoint: u.Hostname(), Port: port,
		Username: "u", Password: "p", InsecureSkipVerify: models.NewEnvBool(true),
	}
	client, err := NewClusterClient(context.Background(), cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	store := NewSnapshotStore()
	coll := NewCollector([]Client{client}, store, time.Second, 10*time.Second, nil)
	coll.CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	if err := reg.Register(NewPromCollector(store)); err != nil {
		t.Fatal(err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"powerscale_up": false,
		"powerscale_cluster_total_capacity_bytes":             false,
		"powerscale_node_memory_used_bytes":                   false,
		"powerscale_quota_usage_bytes":                        false,
		"powerscale_quota_soft_threshold_bytes":               false,
		"powerscale_protocol_operations_per_second":           false,
		"powerscale_node_readonly":                            false,
		"powerscale_node_drives_total":                        false,
		"powerscale_snapshot_used_bytes":                      false,
		"powerscale_synciq_policy_enabled":                    false,
		"powerscale_active_events":                            false,
		"powerscale_dedupe_logical_saved_bytes":               false,
		"powerscale_drive_operations_per_second":              false,
		"powerscale_client_operations_per_second":             false,
		"powerscale_node_power_supplies_total":                false,
		"powerscale_node_temperature_celsius":                 false,
		"powerscale_node_cache_l1_read_hit_bytes_per_second":  false,
		"powerscale_node_cache_l1_read_miss_bytes_per_second": false,
		"powerscale_node_cache_l2_read_hit_bytes_per_second":  false,
		"powerscale_node_cache_l2_read_miss_bytes_per_second": false,
		"powerscale_node_cache_l3_read_hit_bytes_per_second":  false,
		"powerscale_node_cache_l3_read_miss_bytes_per_second": false,
		"powerscale_license_expiration_timestamp_seconds":     false,
		"powerscale_license_active":                           false,
		"powerscale_storagepool_total_capacity_bytes":         false,
		"powerscale_storagepool_used_capacity_bytes":          false,
		"powerscale_storagepool_available_capacity_bytes":     false,
		"powerscale_storagepool_ssd_total_capacity_bytes":     false,
		"powerscale_storagepool_ssd_used_capacity_bytes":      false,
		"powerscale_storagepool_ssd_available_capacity_bytes": false,
		"powerscale_storagepool_hdd_total_capacity_bytes":     false,
		"powerscale_storagepool_hdd_used_capacity_bytes":      false,
		"powerscale_storagepool_hdd_available_capacity_bytes": false,
		"powerscale_workload_operations_per_second":           false,
		"powerscale_workload_in_bytes_per_second":             false,
		"powerscale_workload_out_bytes_per_second":            false,
		"powerscale_workload_cpu_microseconds_per_second":     false,
	}
	for _, mf := range mfs {
		if _, ok := want[mf.GetName()]; ok {
			want[mf.GetName()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing metric %s", name)
		}
	}
}
