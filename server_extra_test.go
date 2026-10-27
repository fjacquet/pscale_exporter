package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/fjacquet/pscale_exporter/internal/models"
	"github.com/fjacquet/pscale_exporter/internal/powerscale"
)

// Slow by design (~15s): with no collector listening, Shutdown waits out shutdownTimeout
// while the OTLP exporter tries to flush.
func TestStartWithOTLPMetricsAndTracingEnabled(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	path := writeConfig(t, dir, port, "1h", "c1")
	// OTLP exporters dial lazily, so no collector needs to be listening.
	extra := `opentelemetry:
  metrics:
    enabled: true
    endpoint: "127.0.0.1:4317"
    insecure: true
    interval: "1h"
  tracing:
    enabled: true
    endpoint: "127.0.0.1:4317"
    insecure: true
    samplingRate: 1.0
`
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, extra...), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t, path)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.otlp == nil {
		t.Error("OTLP metrics exporter not created although enabled")
	}
	if srv.telemetry == nil || srv.tracerProvider == nil {
		t.Error("tracing not initialised although enabled")
	}

	// A reload that changes the cluster set re-registers the OTLP instruments.
	writeConfig(t, dir, port, "1h", "c2")
	b, _ = os.ReadFile(path)
	if err := os.WriteFile(path, append(b, extra...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.ReloadConfig(path); err != nil {
		t.Fatalf("ReloadConfig with OTLP enabled: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	case <-time.After(40 * time.Second):
		t.Fatal("Shutdown with OTLP enabled did not return")
	}
}

func TestDumpSamplesPrintsSortedExpositionLines(t *testing.T) {
	snap := powerscale.BuildSnapshot([]*powerscale.ClusterSnapshot{{
		Cluster: "c1", Up: true,
		Samples: []powerscale.Sample{
			{Name: "powerscale_b", Labels: []powerscale.Label{{Name: "cluster", Value: "c1"}}, Value: 2},
			{Name: "powerscale_a", Labels: []powerscale.Label{{Name: "cluster", Value: "c1"}, {Name: "node", Value: "1"}}, Value: 1},
		},
	}})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	dumpSamples(snap)
	os.Stdout = prev
	_ = w.Close()
	out, _ := io.ReadAll(r)

	want := "powerscale_a{cluster=\"c1\",node=\"1\"} 1\npowerscale_b{cluster=\"c1\"} 2\n"
	if string(out) != want {
		t.Errorf("dumpSamples output:\n%q\nwant:\n%q", out, want)
	}
}

func TestSetupLoggingDebugWritesToFile(t *testing.T) {
	prevLevel := log.GetLevel()
	t.Cleanup(func() { log.SetLevel(prevLevel); log.SetOutput(os.Stdout) })

	var cfg models.Config
	cfg.Server.LogName = filepath.Join(t.TempDir(), "x.log")
	if err := setupLogging(cfg, true); err != nil {
		t.Fatal(err)
	}
	if log.GetLevel() != log.DebugLevel {
		t.Errorf("level = %v, want debug", log.GetLevel())
	}
	log.Debug("marker-line")
	b, err := os.ReadFile(cfg.Server.LogName)
	if err != nil || !strings.Contains(string(b), "marker-line") {
		t.Errorf("log file missing the debug line: %v %q", err, b)
	}
}

func TestWaitForShutdown(t *testing.T) {
	t.Run("server error is returned", func(t *testing.T) {
		ch := make(chan error, 1)
		ch <- errors.New("listen failed")
		if err := waitForShutdown(ch); err == nil || err.Error() != "listen failed" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("SIGTERM is a clean stop", func(t *testing.T) {
		done := make(chan error, 1)
		go func() { done <- waitForShutdown(make(chan error)) }()
		time.Sleep(100 * time.Millisecond) // let signal.Notify register
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("waitForShutdown ignored SIGTERM")
		}
	})
}

func TestBuildClientsKeepsUnreachableClusterAsLazy(t *testing.T) {
	cfg, err := validateConfig(writeConfig(t, t.TempDir(), "9444", "30s", "c1"))
	if err != nil {
		t.Fatal(err)
	}
	clients := buildClients(t.Context(), cfg)
	if len(clients) != 1 || clients[0].Name() != "c1" {
		t.Fatalf("clients = %v, want the unreachable cluster kept", clients)
	}
	if _, ok := clients[0].(*powerscale.LazyClient); !ok {
		t.Errorf("client is %T, want *powerscale.LazyClient", clients[0])
	}
}
