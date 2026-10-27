package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func testConfig() Config {
	return Config{
		Endpoint:       "127.0.0.1:4317",
		Insecure:       true,
		SamplingRate:   1.0,
		ServiceName:    "pscale-exporter-test",
		ServiceVersion: "test",
	}
}

func TestUninitializedManagerIsInert(t *testing.T) {
	m := NewManager(testConfig())
	if m.IsEnabled() {
		t.Fatal("a new manager must not report tracing as enabled")
	}
	if m.TracerProvider() != nil {
		t.Fatal("TracerProvider must be nil before Initialize")
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown on an uninitialized manager: %v", err)
	}
}

func TestInitializeAndShutdown(t *testing.T) {
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	m := NewManager(testConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The gRPC exporter dials lazily, so Initialize succeeds with no collector listening.
	if err := m.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !m.IsEnabled() || m.TracerProvider() == nil {
		t.Fatal("manager should be enabled with a tracer provider after Initialize")
	}
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestSamplerFollowsSamplingRate(t *testing.T) {
	cases := []struct {
		rate float64
		want string
	}{
		{1.0, "AlwaysOnSampler"},
		{2.0, "AlwaysOnSampler"},
		{0.1, "TraceIDRatioBased{0.1}"},
		{0, "TraceIDRatioBased{0}"},
	}
	for _, c := range cases {
		cfg := testConfig()
		cfg.SamplingRate = c.rate
		got := NewManager(cfg).sampler().Description()
		if got != c.want {
			t.Errorf("rate %v: sampler %q, want %q", c.rate, got, c.want)
		}
	}
}

func TestExporterOptionsInsecureFlag(t *testing.T) {
	secure := testConfig()
	secure.Insecure = false
	if n := len(NewManager(secure).exporterOptions()); n != 1 {
		t.Errorf("secure: %d options, want 1 (endpoint only)", n)
	}
	if n := len(NewManager(testConfig()).exporterOptions()); n != 2 {
		t.Errorf("insecure: %d options, want 2 (endpoint + plaintext credentials)", n)
	}
}

func TestResourceCarriesServiceIdentity(t *testing.T) {
	res, err := NewManager(testConfig()).resource()
	if err != nil {
		t.Fatal(err)
	}
	got := res.String()
	for _, want := range []string{"service.name=pscale-exporter-test", "service.version=test"} {
		if !strings.Contains(got, want) {
			t.Errorf("resource %q missing %q", got, want)
		}
	}
}
