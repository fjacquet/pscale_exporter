package powerscale

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	// hwExplained records the cluster+message pairs already logged, so each distinct
	// missing-hardware explanation appears once instead of at every interval.
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

// explainHardwareOnce logs why the node hardware metrics are absent. Empty Fan Speed /
// Node Temperature / Power-Supply panels are the single most confusing thing about running
// this exporter against a virtual cluster, and the cause is only visible in the nodes
// payload. The dedup key is the cluster plus the message itself rather than the cluster
// alone: an unguarded Infof would repeat at every interval (~2900 times a day at 30s), but
// keying on the cluster alone would also silence a cluster whose hardware later changes —
// a node replaced, sensors going silent, a virtual node joining.
func (c *Collector) explainHardwareOnce(cluster string, inv *models.Inventory) {
	if inv == nil || len(inv.Nodes) == 0 {
		return
	}
	msg := explainMissingHardware(inv.Nodes)
	if msg == "" {
		return
	}
	if _, seen := c.hwExplained.LoadOrStore(cluster+"\x00"+msg, struct{}{}); seen {
		return
	}
	log.Infof("cluster %q: %s", cluster, msg)
}

// explainMissingHardware returns an operator-facing sentence naming the metrics a cluster
// cannot produce and why, or "" when every node reports sensors. Virtual nodes are the
// common case (a hypervisor exposes no physical sensors), but a physical node whose sensor
// subsystem returns nothing is reported too — the trigger is the observed absence, not the
// platform.
func explainMissingHardware(nodes []models.Node) string {
	var silent []string
	virtual, identity := 0, ""
	for _, n := range nodes {
		if n.ReportsHardwareSensors() {
			continue
		}
		silent = append(silent, strconv.Itoa(n.LNN))
		if n.VirtualHardware() {
			virtual++
			if identity == "" {
				identity = fmt.Sprintf("series=%s, hwgen=%s, product=%s", n.Series, n.HWGen, n.Product)
			}
		}
	}
	if len(silent) == 0 {
		return ""
	}
	reason := fmt.Sprintf("report no power supplies and no temperature or fan sensors (nodes %s)",
		strings.Join(silent, ","))
	if virtual == len(silent) {
		reason = fmt.Sprintf("report virtual hardware (%s) and expose no physical sensors", identity)
	}
	return fmt.Sprintf("%d/%d nodes %s, so powerscale_node_temperature_celsius, "+
		"powerscale_node_fan_speed_rpm, powerscale_node_power_supplies_total and "+
		"powerscale_node_power_supply_failures are absent for those nodes",
		len(silent), len(nodes), reason)
}
