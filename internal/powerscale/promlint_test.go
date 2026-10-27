package powerscale

import (
	"context"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
)

// Every exposed metric must follow the Prometheus naming conventions (the same linter as
// `promtool check metrics`). Names are frozen by ADR-0007, so a violation has to be caught
// before release rather than after.
func TestMetricsFollowPrometheusNamingConventions(t *testing.T) {
	srv := newMockOneFSAPI(t, 26) // 9.15 exposes the most metrics
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := models.ClusterConfig{
		Name: "clu1", Endpoint: u.Hostname(), Port: port,
		Username: "u", Password: "p", InsecureSkipVerify: models.NewEnvBool(true),
	}
	client, err := NewClusterClient(context.Background(), cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	store := NewSnapshotStore()
	NewCollector([]Client{client}, store, time.Second, 10*time.Second, nil).CollectOnce(context.Background())

	reg := prometheus.NewRegistry()
	if err := reg.Register(NewPromCollector(store)); err != nil {
		t.Fatal(err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := promlint.NewWithMetricFamilies(mfs).Lint()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}
}
