package powerscale

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

func TestCollectClusterSuccess(t *testing.T) {
	fc := &fakeClient{
		name: "clu1",
		inv: &models.Inventory{
			Cluster: models.ClusterInfo{GUID: "G1"},
			Counts:  models.Counts{NFSExports: 2},
		},
		st: &models.Statistics{Current: []models.StatPoint{{Key: "ifs.bytes.total", DevID: 0, Value: 9}}},
	}
	c := NewCollector([]Client{fc}, NewSnapshotStore(), time.Second, time.Second, nil)
	snap := c.CollectOnce(context.Background())

	cs := snap.PerCluster["clu1"]
	if cs == nil || !cs.Up {
		t.Fatalf("cluster not up: %+v", cs)
	}
	if len(snap.SamplesByName("powerscale_cluster_total_capacity_bytes")) != 1 {
		t.Fatal("missing capacity sample")
	}
}

func TestCollectClusterDegradesOnError(t *testing.T) {
	fc := &fakeClient{name: "clu1", err: errors.New("boom")}
	c := NewCollector([]Client{fc}, NewSnapshotStore(), time.Second, time.Second, nil)
	snap := c.CollectOnce(context.Background())

	cs := snap.PerCluster["clu1"]
	if cs == nil || cs.Up {
		t.Fatalf("expected down cluster, got %+v", cs)
	}
	if cs.ScrapeError == "" {
		t.Fatal("expected scrape error recorded")
	}
}
