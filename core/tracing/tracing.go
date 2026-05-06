// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package tracing wires OpenTelemetry into every SovereignStack service
// with a single Init call. Phase G1.
//
// Design intent: zero overhead by default. When OTEL_EXPORTER_OTLP_ENDPOINT
// is unset (the default), Init is a no-op — the global TracerProvider
// stays at the SDK's default (no exporters, no batching, near-zero cost).
// When the env var is set, Init wires up an OTLP/gRPC exporter and
// registers a W3C TraceContext propagator so the gateway can pass
// traceparent headers through to the model containers.
package tracing

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// Shutdown releases the tracing pipeline. Call from main on signal.
type Shutdown func(context.Context) error

// noop is what Init returns when tracing is disabled.
var noop Shutdown = func(context.Context) error { return nil }

// Init configures the global TracerProvider for serviceName. Returns a
// Shutdown the caller should defer. When OTEL_EXPORTER_OTLP_ENDPOINT is
// empty, returns a no-op shutdown without touching the SDK.
//
// Honoured env vars (compatible with the OTel spec):
//
//	OTEL_EXPORTER_OTLP_ENDPOINT  e.g. "localhost:4317"
//	OTEL_EXPORTER_OTLP_INSECURE  "true" to skip TLS (dev / Jaeger)
//	OTEL_SERVICE_NAME            overrides serviceName
func Init(ctx context.Context, serviceName string) (Shutdown, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return noop, nil
	}
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		serviceName = name
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true" {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return noop, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}, nil
}
