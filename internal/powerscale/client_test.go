package powerscale

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
	log "github.com/sirupsen/logrus"
)

func newTestClient(t *testing.T) *ClusterClient {
	t.Helper()
	return newTestClientOpts(t, "", false)
}

func newTestClientDump(t *testing.T, dumpDir string) *ClusterClient {
	t.Helper()
	return newTestClientOpts(t, dumpDir, false)
}

func newTestClientOpts(t *testing.T, dumpDir string, trace bool) *ClusterClient {
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
	c, err := NewClusterClient(context.Background(), cfg, dumpDir, trace)
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

// TestClientDumpResponses verifies --dump-dir behavior: every fetched endpoint lands as
// <dir>/<cluster>/<sanitized_path>.json containing the verbatim (valid-JSON) body, ready
// to be shipped back from a remote site and dropped into testdata/.
func TestClientDumpResponses(t *testing.T) {
	dir := t.TempDir()
	c := newTestClientDump(t, dir)
	if _, err := c.GetInventory(context.Background()); err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	if _, err := c.GetStatistics(context.Background()); err != nil {
		t.Fatalf("GetStatistics: %v", err)
	}
	for _, name := range []string{
		"platform_3_cluster_config.json",
		"platform_3_cluster_nodes.json",
		"platform_1_quota_quotas.json",
		"platform_1_statistics_current.json",
		"platform_2_statistics_summary_protocol.json",
	} {
		b, err := os.ReadFile(filepath.Join(dir, "clu1", name))
		if err != nil {
			t.Fatalf("dump file %s: %v", name, err)
		}
		if !json.Valid(b) {
			t.Fatalf("dump file %s is not valid JSON", name)
		}
	}
}

// TestClientTraceLogsBodiesNeverHeaders verifies the --trace token-safety contract:
// response bodies (plus method/URL, and the status of failed requests) are logged,
// while header material — where the OneFS session credentials live — never appears.
// The /session/1/session login happens inside gopowerscale during NewClusterClient,
// so capturing the log from before construction proves the auth exchange (the mock
// sets an isisessid=test-session cookie) leaks nothing into the trace.
func TestClientTraceLogsBodiesNeverHeaders(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	c := newTestClientOpts(t, "", true)
	if _, err := c.GetInventory(context.Background()); err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	var b []byte
	if err := c.getRaw(context.Background(), "platform/1/no/such/endpoint", &b); err == nil {
		t.Fatal("expected error for unknown endpoint")
	}

	out := buf.String()
	if !strings.Contains(out, "API trace:") {
		t.Fatal("expected API trace output, got none")
	}
	if !strings.Contains(out, "000abc000def") { // cluster GUID, proves the body is logged
		t.Error("trace output does not contain response body content")
	}
	if !strings.Contains(out, "request failed") {
		t.Error("failed request was not traced")
	}
	for _, leak := range []string{"isisessid", "test-session", "Cookie"} {
		if strings.Contains(out, leak) {
			t.Errorf("trace output leaked header material %q", leak)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("platform/1/statistics/current"); got != "platform_1_statistics_current" {
		t.Fatalf("sanitizeFilename: %q", got)
	}
	if got := sanitizeFilename("../escape attempt"); got != ".._escape_attempt" {
		t.Fatalf("sanitizeFilename traversal: %q", got)
	}
	for _, hazard := range []string{"..", ".", ""} {
		if got := sanitizeFilename(hazard); got != "_" {
			t.Fatalf("sanitizeFilename(%q) = %q, want _", hazard, got)
		}
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
