package powerscale

import (
	"context"
	"net/url"
	"strconv"
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

func newTestClient(t *testing.T) *ClusterClient {
	t.Helper()
	srv := newMockOneFS(t)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := models.ClusterConfig{
		Name:               "clu1",
		Endpoint:           u.Hostname(),
		Port:               port,
		Username:           "u",
		Password:           "p",
		InsecureSkipVerify: true,
	}
	c, err := NewClusterClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestClientAPIVersion(t *testing.T) {
	c := newTestClient(t)
	v, err := c.APIVersion(context.Background())
	if err != nil || v != 16 {
		t.Fatalf("APIVersion = %d err=%v", v, err)
	}
}

func TestClientGetInventory(t *testing.T) {
	c := newTestClient(t)
	inv, err := c.GetInventory(context.Background())
	if err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	if inv.Cluster.GUID != "000abc000def" {
		t.Fatalf("cluster guid: %q", inv.Cluster.GUID)
	}
	if len(inv.Nodes) != 2 {
		t.Fatalf("nodes: %d", len(inv.Nodes))
	}
	if len(inv.Quotas) != 1 || inv.Quotas[0].UsageBytes != 100 {
		t.Fatalf("quotas: %+v", inv.Quotas)
	}
	if inv.Counts.NFSExports != 1 || inv.Counts.SMBShares != 1 || inv.Counts.Snapshots != 1 {
		t.Fatalf("counts: %+v", inv.Counts)
	}
}

func TestClientGetStatistics(t *testing.T) {
	c := newTestClient(t)
	st, err := c.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics: %v", err)
	}
	if len(st.Current) != 4 {
		t.Fatalf("current stats: %d", len(st.Current))
	}
	if len(st.Proto) != 1 || st.Proto[0].OperationRate != 12 {
		t.Fatalf("proto stats: %+v", st.Proto)
	}
}
