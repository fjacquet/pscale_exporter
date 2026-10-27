package powerscale

import (
	"context"
	"sync"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

// LazyClient stands in for a cluster whose session could not be created at startup. It
// retries the creation on every call until it succeeds, so the collector keeps the
// cluster in the snapshot (reported down, with the creation error) and picks it up as
// soon as it becomes reachable, instead of dropping it until the next reload or restart.
type LazyClient struct {
	name   string
	create func(context.Context) (Client, error)

	mu    sync.Mutex
	inner Client
}

// NewLazyClient returns a Client named name that builds its real client with create.
func NewLazyClient(name string, create func(context.Context) (Client, error)) *LazyClient {
	return &LazyClient{name: name, create: create}
}

func (l *LazyClient) get(ctx context.Context) (Client, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inner != nil {
		return l.inner, nil
	}
	c, err := l.create(ctx)
	if err != nil {
		return nil, err
	}
	l.inner = c
	return c, nil
}

// Name returns the configured cluster name.
func (l *LazyClient) Name() string { return l.name }

// APIVersion delegates to the real client, creating it first if needed.
func (l *LazyClient) APIVersion(ctx context.Context) (int, error) {
	c, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	return c.APIVersion(ctx)
}

// GetInventory delegates to the real client, creating it first if needed.
func (l *LazyClient) GetInventory(ctx context.Context) (*models.Inventory, error) {
	c, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetInventory(ctx)
}

// GetStatistics delegates to the real client, creating it first if needed.
func (l *LazyClient) GetStatistics(ctx context.Context) (*models.Statistics, error) {
	c, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetStatistics(ctx)
}

// Close closes the real client if it was ever created.
func (l *LazyClient) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inner == nil {
		return nil
	}
	return l.inner.Close()
}
