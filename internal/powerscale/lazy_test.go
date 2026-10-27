package powerscale

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

// flakyCreate fails the first `failures` attempts, then returns a working client.
func flakyCreate(failures int, attempts *int) func(context.Context) (Client, error) {
	return func(context.Context) (Client, error) {
		*attempts++
		if *attempts <= failures {
			return nil, errors.New("auth failed: connection refused")
		}
		return &fakeClient{name: "c1", inv: &models.Inventory{}, st: &models.Statistics{}}, nil
	}
}

func TestLazyClientReportsCreationErrorThenRecovers(t *testing.T) {
	attempts := 0
	lc := NewLazyClient("c1", flakyCreate(2, &attempts))

	if lc.Name() != "c1" {
		t.Fatalf("Name = %q", lc.Name())
	}
	if _, err := lc.GetInventory(context.Background()); err == nil {
		t.Fatal("expected the creation error from the first call")
	}
	if _, err := lc.GetStatistics(context.Background()); err == nil {
		t.Fatal("expected the creation error from the second call")
	}
	if _, err := lc.GetInventory(context.Background()); err != nil {
		t.Fatalf("third attempt should succeed: %v", err)
	}
	// Once created, the client is reused: no further creation attempts.
	if _, err := lc.GetStatistics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("create called %d times, want 3", attempts)
	}
	if v, err := lc.APIVersion(context.Background()); err != nil || v != 16 {
		t.Fatalf("APIVersion = %d, %v; want the inner client's value", v, err)
	}
}

func TestLazyClientCloseBeforeAndAfterCreation(t *testing.T) {
	attempts := 0
	lc := NewLazyClient("c1", flakyCreate(0, &attempts))
	if err := lc.Close(); err != nil {
		t.Fatalf("Close before creation: %v", err)
	}
	if _, err := lc.GetInventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lc.Close(); err != nil {
		t.Fatalf("Close after creation: %v", err)
	}
}

// The reason the type exists: a cluster that cannot be reached at startup must still
// show up in the snapshot as down, and come up once it becomes reachable.
func TestCollectorReportsUnreachableClusterDownThenUp(t *testing.T) {
	attempts := 0
	store := NewSnapshotStore()
	// A down cluster costs two creation attempts per cycle: APIVersion, then GetInventory.
	col := NewCollector([]Client{NewLazyClient("c1", flakyCreate(2, &attempts))},
		store, time.Second, 2*time.Second, nil)

	snap := col.CollectOnce(context.Background())
	if len(snap.PerCluster) != 1 {
		t.Fatalf("clusters in snapshot = %d, want 1 (the down cluster must not vanish)", len(snap.PerCluster))
	}
	cs := snap.PerCluster["c1"]
	if cs == nil || cs.Cluster != "c1" || cs.Up || cs.ScrapeError == "" {
		t.Fatalf("down cluster = %+v, want Cluster=c1 Up=false with a ScrapeError", cs)
	}

	snap = col.CollectOnce(context.Background())
	if cs := snap.PerCluster["c1"]; cs == nil || !cs.Up || cs.ScrapeError != "" {
		t.Fatalf("after recovery = %+v, want Up=true", cs)
	}
}
