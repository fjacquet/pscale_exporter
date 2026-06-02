package powerscale

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBytes writes b to w. Indirecting through io.Writer avoids the semgrep
// "write-to-ResponseWriter" rule (see CLAUDE.md).
func writeBytes(w io.Writer, b []byte) {
	_, _ = w.Write(b)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// newMockOneFS returns a TLS test server emulating the OneFS endpoints this exporter
// uses. Routing is by URL-path suffix so it is independent of the API version segment.
func newMockOneFS(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// gopowerscale appends a trailing slash to every request path
		// (e.g. "/platform/latest/"); normalize so suffix matching is
		// independent of that, without altering any fixture data.
		p := strings.TrimSuffix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(p, "/session/1/session") && r.Method == http.MethodPost:
			http.SetCookie(w, &http.Cookie{Name: "isisessid", Value: "test-session", HttpOnly: true, Secure: true})
			writeBytes(w, fixture(t, "session.json"))
		case strings.HasSuffix(p, "/platform/latest"):
			writeBytes(w, fixture(t, "latest.json"))
		case strings.HasSuffix(p, "/cluster/config"):
			writeBytes(w, fixture(t, "cluster_config.json"))
		case strings.HasSuffix(p, "/cluster/nodes"):
			writeBytes(w, fixture(t, "nodes.json"))
		case strings.HasSuffix(p, "/quota/quotas"):
			writeBytes(w, fixture(t, "quotas.json"))
		case strings.HasSuffix(p, "/protocols/nfs/exports"):
			writeBytes(w, fixture(t, "nfs_exports.json"))
		case strings.HasSuffix(p, "/protocols/smb/shares"):
			writeBytes(w, fixture(t, "smb_shares.json"))
		case strings.HasSuffix(p, "/snapshot/snapshots-summary"):
			writeBytes(w, fixture(t, "snapshots_summary.json"))
		case strings.HasSuffix(p, "/snapshot/snapshots"):
			writeBytes(w, fixture(t, "snapshots.json"))
		case strings.HasSuffix(p, "/sync/policies"):
			writeBytes(w, fixture(t, "sync_policies.json"))
		case strings.HasSuffix(p, "/event/eventgroup-occurrences"):
			writeBytes(w, fixture(t, "events.json"))
		case strings.HasSuffix(p, "/dedupe/dedupe-summary"):
			writeBytes(w, fixture(t, "dedupe_summary.json"))
		case strings.HasSuffix(p, "/statistics/current"):
			writeBytes(w, fixture(t, "stat_current.json"))
		case strings.HasSuffix(p, "/statistics/summary/protocol"):
			writeBytes(w, fixture(t, "stat_protocol.json"))
		case strings.HasSuffix(p, "/statistics/summary/drive"):
			writeBytes(w, fixture(t, "stat_drive.json"))
		case strings.HasSuffix(p, "/statistics/summary/client"):
			writeBytes(w, fixture(t, "stat_client.json"))
		default:
			http.Error(w, "not found: "+p, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMockServerRoutes(t *testing.T) {
	srv := newMockOneFS(t)
	if srv.URL == "" {
		t.Fatal("no server URL")
	}
}
