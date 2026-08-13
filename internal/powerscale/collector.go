package powerscale

import (
	"context"
	"sync"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

// Collector runs the background collection loop: every interval it polls all clusters
// in parallel and publishes a fresh Snapshot. One cluster's failure does not affect
// the others (graceful degradation).
type Collector struct {
	clients  []Client
	store    *SnapshotStore
	interval time.Duration
	timeout  time.Duration
	tracing  *TracerWrapper
	// hwExplained records the clusters whose missing-hardware explanation has already
	// been logged, so the message appears once per cluster instead of every interval.
	hwExplained sync.Map
}

// NewCollector creates a collection loop over the given per-cluster clients.
func NewCollector(clients []Client, store *SnapshotStore, interval, timeout time.Duration, tp trace.TracerProvider) *Collector {
	return &Collector{
		clients:  clients,
		store:    store,
		interval: interval,
		timeout:  timeout,
		tracing:  NewTracerWrapper(tp, "pscale-exporter/collector"),
	}
}

// CollectOnce runs a single collection cycle and publishes the result.
func (c *Collector) CollectOnce(ctx context.Context) *Snapshot {
	snap := c.collectAll(ctx)
	c.store.Store(snap)
	return snap
}

// Run drives the collection loop until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.store.Store(c.collectAll(ctx))
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) *Snapshot {
	ctx, span := c.tracing.StartSpan(ctx, "collect.cycle", trace.SpanKindInternal)
	defer span.End()

	results := make([]*ClusterSnapshot, len(c.clients))
	g, gctx := errgroup.WithContext(ctx)
	for i, client := range c.clients {
		i, client := i, client
		g.Go(func() error {
			results[i] = c.collectCluster(gctx, client)
			return nil
		})
	}
	_ = g.Wait()
	return BuildSnapshot(results)
}

func (c *Collector) collectCluster(ctx context.Context, client Client) *ClusterSnapshot {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cs := &ClusterSnapshot{Cluster: client.Name(), LastScrape: time.Now()}

	if v, err := client.APIVersion(ctx); err == nil {
		cs.APIVersion = v
	}

	inv, err := client.GetInventory(ctx)
	if err != nil {
		log.Warnf("cluster %q: inventory fetch failed: %v", client.Name(), err)
		cs.ScrapeError = err.Error()
		return cs
	}
	stats, err := client.GetStatistics(ctx)
	if err != nil {
		log.Warnf("cluster %q: statistics fetch failed: %v", client.Name(), err)
		cs.ScrapeError = err.Error()
		return cs
	}
	c.explainHardwareOnce(client.Name(), inv)
	cs.Samples = BuildSamples(client.Name(), inv, stats)
	cs.Up = true
	return cs
}

// explainHardwareOnce logs, at most once per cluster, why the node hardware metrics are
// absent. Empty Fan Speed / Node Temperature / Power-Supply panels are the single most
// confusing thing about running this exporter against a virtual cluster, and the cause is
// only visible in the nodes payload.
func (c *Collector) explainHardwareOnce(cluster string, inv *models.Inventory) {
	if inv == nil || len(inv.Nodes) == 0 {
		return
	}
	if _, seen := c.hwExplained.LoadOrStore(cluster, struct{}{}); seen {
		return
	}
	if msg := ExplainMissingHardware(inv.Nodes); msg != "" {
		log.Infof("cluster %q: %s", cluster, msg)
	}
}
