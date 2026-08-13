package powerscale

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
	log "github.com/sirupsen/logrus"
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

// TestExplainHardwareLogsOncePerCluster pins the guard that keeps the missing-hardware
// explanation out of every collection cycle: at a 30s interval an unguarded Infof would
// repeat it ~2900 times a day.
func TestExplainHardwareLogsOncePerCluster(t *testing.T) {
	virtual := models.Node{LNN: 1, Series: "virtual_series", HWGen: "VMware", Product: "SIMULATOR-1U"}
	fc := &fakeClient{
		name: "clu1",
		inv:  &models.Inventory{Cluster: models.ClusterInfo{GUID: "G1"}, Nodes: []models.Node{virtual}},
		st:   &models.Statistics{},
	}
	c := NewCollector([]Client{fc}, NewSnapshotStore(), time.Second, time.Second, nil)

	var logged int
	hook := logCountHook{substr: "report virtual hardware", n: &logged}
	log.AddHook(hook)
	defer func() { log.StandardLogger().ReplaceHooks(nil) }()

	for i := 0; i < 3; i++ {
		c.CollectOnce(context.Background())
	}
	if logged != 1 {
		t.Fatalf("explanation logged %d times across 3 cycles, want 1", logged)
	}
}

// logCountHook counts logrus entries whose message contains substr.
type logCountHook struct {
	substr string
	n      *int
}

func (h logCountHook) Levels() []log.Level { return log.AllLevels }
func (h logCountHook) Fire(e *log.Entry) error {
	if strings.Contains(e.Message, h.substr) {
		*h.n++
	}
	return nil
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
