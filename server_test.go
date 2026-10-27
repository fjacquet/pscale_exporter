package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/models"
	"github.com/fjacquet/pscale_exporter/internal/utils"
)

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
}

// writeConfig writes a minimal valid config whose single cluster refuses connections
// immediately (127.0.0.1:1), so client init fails fast and the cluster is marked down.
func writeConfig(t *testing.T, dir, port, interval, clusterName string) string {
	t.Helper()
	body := fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: %q
  uri: "/metrics"
collection:
  interval: %q
  timeout: "2s"
clusters:
  - name: %s
    endpoint: "127.0.0.1"
    port: 1
    username: "monitor"
    password: "secret"
`, port, interval, clusterName)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestServer(t *testing.T, path string) *Server {
	t.Helper()
	cfg, err := validateConfig(path)
	if err != nil {
		t.Fatalf("validateConfig: %v", err)
	}
	if err := utils.ResolveSecrets(cfg); err != nil {
		t.Fatal(err)
	}
	return NewServer(models.NewSafeConfig(cfg, utils.ResolveSecrets), path)
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestValidateConfig(t *testing.T) {
	dir := t.TempDir()

	if _, err := validateConfig(filepath.Join(dir, "missing.yaml")); err == nil ||
		!strings.Contains(err.Error(), "config file not found") {
		t.Errorf("missing file: err = %v", err)
	}

	noClusters := filepath.Join(dir, "no-clusters.yaml")
	if err := os.WriteFile(noClusters, []byte("server:\n  host: \"127.0.0.1\"\n  port: \"9444\"\n  uri: \"/metrics\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateConfig(noClusters); err == nil || !strings.Contains(err.Error(), "invalid configuration") {
		t.Errorf("no clusters: err = %v", err)
	}

	cfg, err := validateConfig(writeConfig(t, dir, "9444", "30s", "c1"))
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if len(cfg.Clusters) != 1 || cfg.Clusters[0].Name != "c1" {
		t.Errorf("clusters = %+v", cfg.Clusters)
	}
}

func TestServerServesEndpointsAndShutsDown(t *testing.T) {
	port := freePort(t)
	srv := newTestServer(t, writeConfig(t, t.TempDir(), port, "1h", "c1"))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	base := "http://127.0.0.1:" + port

	deadline := time.Now().Add(5 * time.Second)
	for {
		if code, _ := get(t, base+"/livez"); code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start listening")
		}
		time.Sleep(25 * time.Millisecond)
	}

	for _, path := range []string{"/livez", "/readyz", "/health"} {
		if code, _ := get(t, base+path); code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, code)
		}
	}

	code, body := get(t, base+"/metrics")
	if code != http.StatusOK {
		t.Fatalf("/metrics status = %d", code)
	}
	if !strings.Contains(body, "pscale_exporter_build_info") {
		t.Error("/metrics missing pscale_exporter_build_info")
	}

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if code, _ := get(t, base+"/livez"); code != 0 {
		t.Errorf("server still answering after Shutdown (status %d)", code)
	}
}

func TestReloadConfigKeepsRunningConfigWhenFileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "9444", "30s", "c1")
	srv := newTestServer(t, path)

	if err := os.WriteFile(path, []byte("server: [not a mapping\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.ReloadConfig(path); err == nil {
		t.Fatal("expected an error for an invalid config file")
	}
	if got := srv.cfg.Get().Collection.Interval; got != "30s" {
		t.Errorf("running config changed to interval %q after a failed reload", got)
	}
}

func TestReloadConfigAppliesChangesAndRebuildsOnClusterChange(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "9444", "30s", "c1")
	srv := newTestServer(t, path)
	t.Cleanup(func() { _ = srv.Shutdown() })

	// Same clusters, new interval: applied without rebuilding the pool.
	writeConfig(t, dir, "9444", "45s", "c1")
	if err := srv.ReloadConfig(path); err != nil {
		t.Fatal(err)
	}
	if got := srv.cfg.Get().Collection.Interval; got != "45s" {
		t.Errorf("interval = %q, want 45s", got)
	}
	srv.mu.Lock()
	rebuilt := srv.collector != nil
	srv.mu.Unlock()
	if rebuilt {
		t.Error("collection was rebuilt although the cluster set did not change")
	}

	// A different cluster: the collection loop restarts.
	writeConfig(t, dir, "9444", "45s", "c2")
	if err := srv.ReloadConfig(path); err != nil {
		t.Fatal(err)
	}
	if got := srv.cfg.Get().Clusters[0].Name; got != "c2" {
		t.Errorf("cluster = %q, want c2", got)
	}
	srv.mu.Lock()
	running := srv.collector != nil && srv.collectCancel != nil
	srv.mu.Unlock()
	if !running {
		t.Error("collection loop not restarted after the cluster set changed")
	}
}

func TestOnceRunsASingleCycleWithoutALoop(t *testing.T) {
	prev := once
	once = true
	t.Cleanup(func() { once = prev })

	port := freePort(t)
	srv := newTestServer(t, writeConfig(t, t.TempDir(), port, "1h", "c1"))
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown() })

	srv.mu.Lock()
	hasLoop := srv.collectCancel != nil
	srv.mu.Unlock()
	if hasLoop {
		t.Error("--once must not start the background collection loop")
	}
}

// A cluster whose client cannot be created at startup is reported down (LazyClient), not
// dropped: otherwise powerscale_up{cluster} 0 never appears and PowerScaleClusterDown
// cannot fire.
func TestUnreachableClusterIsReportedDown(t *testing.T) {
	port := freePort(t)
	srv := newTestServer(t, writeConfig(t, t.TempDir(), port, "1h", "c1"))
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown() })

	_, body := get(t, "http://127.0.0.1:"+port+"/metrics")
	if !strings.Contains(body, `powerscale_up{cluster="c1"} 0`) {
		t.Errorf("an unreachable cluster should be reported as powerscale_up 0, got:\n%s", body)
	}
}
