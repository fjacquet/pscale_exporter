package powerscale

import (
	"context"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

// StubClient is a placeholder client used until the real ClusterClient lands. It emits
// one synthetic capacity sample so the pipeline is observable end to end.
type StubClient struct{ name string }

// NewStubClient returns a stub client for the named cluster.
func NewStubClient(name string) *StubClient { return &StubClient{name: name} }

func (s *StubClient) Name() string                            { return s.name }
func (s *StubClient) APIVersion(context.Context) (int, error) { return 0, nil }
func (s *StubClient) GetInventory(context.Context) (*models.Inventory, error) {
	return &models.Inventory{Cluster: models.ClusterInfo{GUID: "stub"}}, nil
}
func (s *StubClient) GetStatistics(context.Context) (*models.Statistics, error) {
	return &models.Statistics{Current: []models.StatPoint{{Key: "ifs.bytes.total", DevID: 0, Value: 1}}}, nil
}
func (s *StubClient) Close() error { return nil }
