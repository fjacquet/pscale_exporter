package powerscale

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromCollectorEmitsUpAndSamples(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(BuildSnapshot([]*ClusterSnapshot{
		{
			Cluster: "clu1", Up: true, APIVersion: 16,
			Samples: []Sample{{
				Name:   "powerscale_cluster_total_capacity_bytes",
				Labels: []Label{{"cluster", "clu1"}, {"cluster_id", "G1"}},
				Value:  5000,
			}},
		},
	}))

	reg := prometheus.NewRegistry()
	if err := reg.Register(NewPromCollector(store)); err != nil {
		t.Fatal(err)
	}

	expected := `
# HELP powerscale_up 1 if the cluster was scraped successfully, 0 otherwise
# TYPE powerscale_up gauge
powerscale_up{cluster="clu1"} 1
`
	if err := testutil.CollectAndCompare(NewPromCollector(store), strings.NewReader(expected), "powerscale_up"); err != nil {
		t.Fatal(err)
	}
}
