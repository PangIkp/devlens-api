package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

type TracingConfig struct {
	Enabled         bool
	ServiceName     string
	ExporterEndpoint string
	Insecure        bool
	SampleRatio     float64
}

type Tracing struct {
	provider *sdktrace.TracerProvider
}

func SetupTracing(ctx context.Context, cfg TracingConfig) (*Tracing, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if !cfg.Enabled || cfg.ExporterEndpoint == "" {
		return &Tracing{}, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.ExporterEndpoint),
		otlptracehttp.WithTimeout(5*time.Second),
		func() otlptracehttp.Option {
			if cfg.Insecure {
				return otlptracehttp.WithInsecure()
			}
			return otlptracehttp.WithURLPath("/v1/traces")
		}(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	sampleRatio := cfg.SampleRatio
	if sampleRatio <= 0 || sampleRatio > 1 {
		sampleRatio = 1
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		)),
	)
	otel.SetTracerProvider(provider)

	return &Tracing{provider: provider}, nil
}

func (t *Tracing) Shutdown(ctx context.Context) error {
	if t == nil || t.provider == nil {
		return nil
	}
	return t.provider.Shutdown(ctx)
}
