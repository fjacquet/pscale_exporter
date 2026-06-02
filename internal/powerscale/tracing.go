package powerscale

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracerWrapper provides nil-safe tracing via a noop provider default, so span
// methods are always safe to call without nil checks.
type TracerWrapper struct {
	tracer trace.Tracer
}

// NewTracerWrapper builds a TracerWrapper; a nil provider yields a noop tracer.
func NewTracerWrapper(tp trace.TracerProvider, instrumentationName string) *TracerWrapper {
	if tp == nil {
		tp = noop.NewTracerProvider()
	}
	return &TracerWrapper{tracer: tp.Tracer(instrumentationName)}
}

// StartSpan starts a span of the given kind; always returns a valid span.
func (w *TracerWrapper) StartSpan(ctx context.Context, operation string, kind trace.SpanKind) (context.Context, trace.Span) {
	return w.tracer.Start(ctx, operation, trace.WithSpanKind(kind))
}
