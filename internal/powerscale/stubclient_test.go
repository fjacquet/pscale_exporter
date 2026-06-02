package powerscale

import (
	"context"
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

// fakeClient is the test double used across collector tests.
type fakeClient struct {
	name string
	inv  *models.Inventory
	st   *models.Statistics
	err  error
}

func (f *fakeClient) Name() string                            { return f.name }
func (f *fakeClient) APIVersion(context.Context) (int, error) { return 16, nil }
func (f *fakeClient) GetInventory(context.Context) (*models.Inventory, error) {
	return f.inv, f.err
}
func (f *fakeClient) GetStatistics(context.Context) (*models.Statistics, error) {
	return f.st, f.err
}
func (f *fakeClient) Close() error { return nil }

func TestFakeClientSatisfiesInterface(t *testing.T) {
	var _ Client = &fakeClient{name: "c1"}
}
