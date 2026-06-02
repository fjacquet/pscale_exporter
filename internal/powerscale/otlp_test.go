package powerscale

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestOTLPObservesSamples(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(BuildSnapshot([]*ClusterSnapshot{
		{Cluster: "clu1", Up: true, Samples: []Sample{{
			Name:   "powerscale_cluster_total_capacity_bytes",
			Labels: []Label{{"cluster", "clu1"}},
			Value:  5000,
		}}},
	}))

	reader := sdkmetric.NewManualReader()
	exp := newOTLPExporter(reader, store, "test")
	if err := exp.EnsureInstruments(); err != nil {
		t.Fatal(err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "powerscale_cluster_total_capacity_bytes" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("capacity metric not observed via OTLP")
	}
}
