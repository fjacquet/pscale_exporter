// Package telemetry manages the optional OpenTelemetry tracer provider used to
// diagnose slow collection cycles. Metric export lives in the powerscale package.
package telemetry

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds tracer settings.
type Config struct {
	Endpoint       string
	Insecure       bool
	SamplingRate   float64
	ServiceName    string
	ServiceVersion string
}

// Manager owns the tracer provider lifecycle.
type Manager struct {
	enabled        bool
	tracerProvider *sdktrace.TracerProvider
	config         Config
}

// NewManager returns an uninitialized Manager.
func NewManager(cfg Config) *Manager {
	return &Manager{config: cfg}
}

// Initialize creates the OTLP trace exporter and registers the tracer provider.
// It returns an error on exporter or resource construction failure (the caller
// decides whether that is fatal).
func (m *Manager) Initialize(ctx context.Context) error {
	exp, err := otlptracegrpc.New(ctx, m.exporterOptions()...)
	if err != nil {
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	res, err := m.resource()
	if err != nil {
		return fmt.Errorf("failed to create telemetry resource: %w", err)
	}

	m.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(m.sampler()),
	)
	otel.SetTracerProvider(m.tracerProvider)
	m.enabled = true

	logrus.Infof("OpenTelemetry tracing initialized (endpoint: %s, sampling: %.2f)",
		m.config.Endpoint, m.config.SamplingRate)
	return nil
}

func (m *Manager) exporterOptions() []otlptracegrpc.Option {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(m.config.Endpoint)}
	if m.config.Insecure {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
	}
	return opts
}

func (m *Manager) resource() (*resource.Resource, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return resource.New(context.Background(), resource.WithAttributes(
		semconv.ServiceName(m.config.ServiceName),
		semconv.ServiceVersion(m.config.ServiceVersion),
		semconv.HostName(hostname),
	))
}

func (m *Manager) sampler() sdktrace.Sampler {
	if m.config.SamplingRate >= 1.0 {
		return sdktrace.AlwaysSample()
	}
	return sdktrace.TraceIDRatioBased(m.config.SamplingRate)
}

// Shutdown flushes and stops the tracer provider.
func (m *Manager) Shutdown(ctx context.Context) error {
	if !m.enabled || m.tracerProvider == nil {
		return nil
	}
	if err := m.tracerProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown tracer provider: %w", err)
	}
	return nil
}

// IsEnabled reports whether tracing initialized successfully.
func (m *Manager) IsEnabled() bool { return m.enabled }

// TracerProvider returns the provider, or nil if not initialized.
func (m *Manager) TracerProvider() trace.TracerProvider {
	if m.tracerProvider == nil {
		return nil
	}
	return m.tracerProvider
}
