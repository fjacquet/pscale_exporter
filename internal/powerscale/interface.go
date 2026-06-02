// Package powerscale provides the OneFS platform API client, data collection, and the
// dual (Prometheus + OTLP) metric export paths for Dell PowerScale clusters.
package powerscale

import (
	"context"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

// Client is the per-cluster OneFS API client abstraction. Satisfied by ClusterClient
// and mocked in tests so the collector can run without a live cluster.
type Client interface {
	// Name returns the configured cluster name (the `cluster` label).
	Name() string
	// APIVersion returns the detected platform API version (cached after first call).
	APIVersion(ctx context.Context) (int, error)
	// GetInventory fetches typed resources: cluster info, nodes, quotas, counts.
	GetInventory(ctx context.Context) (*models.Inventory, error)
	// GetStatistics fetches the curated statistics keys and protocol summary.
	GetStatistics(ctx context.Context) (*models.Statistics, error)
	// Close releases HTTP resources.
	Close() error
}
