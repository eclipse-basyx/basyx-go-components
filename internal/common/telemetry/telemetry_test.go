/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package telemetry

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestConfigureLeavesTelemetryDisabledByDefault(t *testing.T) {
	clearTelemetryEnvironment(t)
	previousProvider := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousPropagator := otel.GetTextMapPropagator()

	runtime, err := Configure(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	if runtime.Enabled() {
		t.Fatal("telemetry unexpectedly enabled")
	}
	if otel.GetTracerProvider() != previousProvider {
		t.Fatal("disabled configuration replaced the tracer provider")
	}
	if otel.GetMeterProvider() != previousMeter {
		t.Fatal("disabled configuration replaced the meter provider")
	}
	if otel.GetTextMapPropagator() != previousPropagator {
		t.Fatal("disabled configuration replaced the propagator")
	}
}

func TestConfigureDisablesTelemetryForNoneAndSDKDisabled(t *testing.T) {
	for _, test := range []struct {
		name     string
		exporter string
		disabled string
	}{
		{name: "empty exporter", exporter: ""},
		{name: "none exporter", exporter: "none"},
		{name: "SDK disabled", exporter: "unsupported", disabled: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearTelemetryEnvironment(t)
			t.Setenv("OTEL_TRACES_EXPORTER", test.exporter)
			if test.disabled != "" {
				t.Setenv("OTEL_SDK_DISABLED", test.disabled)
			}

			runtime, err := Configure(t.Context(), "testservice")
			if err != nil {
				t.Fatalf("configure telemetry: %v", err)
			}
			if runtime.Enabled() {
				t.Fatal("telemetry unexpectedly enabled")
			}
		})
	}
}

func TestConfigureEnablesConsoleExporterAndRestoresGlobals(t *testing.T) {
	clearTelemetryEnvironment(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "console")
	originalProvider := otel.GetTracerProvider()
	originalPropagator := otel.GetTextMapPropagator()
	originalErrorHandler := otel.GetErrorHandler()
	previousProvider := nooptrace.NewTracerProvider()
	previousPropagator := propagation.NewCompositeTextMapPropagator(propagation.Baggage{})
	previousErrorHandler := &testErrorHandler{}
	otel.SetTracerProvider(previousProvider)
	otel.SetTextMapPropagator(previousPropagator)
	otel.SetErrorHandler(previousErrorHandler)
	t.Cleanup(func() {
		otel.SetTracerProvider(originalProvider)
		otel.SetTextMapPropagator(originalPropagator)
		otel.SetErrorHandler(originalErrorHandler)
	})

	runtime, err := Configure(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	if !runtime.Enabled() {
		t.Fatal("telemetry was not enabled")
	}
	if otel.GetTracerProvider() == previousProvider {
		t.Fatal("tracer provider was not installed")
	}

	runtime.Shutdown(t.Context())

	if otel.GetTracerProvider() != previousProvider {
		t.Fatal("tracer provider was not restored")
	}
	if got, want := otel.GetTextMapPropagator().Fields(), previousPropagator.Fields(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("propagator was not restored: got %v want %v", got, want)
	}
	if otel.GetErrorHandler() != previousErrorHandler {
		t.Fatal("error handler was not restored")
	}
}

func TestConfigureEnablesOTLPExporter(t *testing.T) {
	clearTelemetryEnvironment(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")

	runtime, err := Configure(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	t.Cleanup(func() { runtime.Shutdown(t.Context()) })
	if !runtime.Enabled() {
		t.Fatal("OTLP telemetry was not enabled")
	}
}

func TestConfigureEnablesMetricsWithoutChangingTracing(t *testing.T) {
	clearTelemetryEnvironment(t)
	t.Setenv("OTEL_METRICS_EXPORTER", "console")
	originalMeter := otel.GetMeterProvider()
	originalTracer := otel.GetTracerProvider()

	runtime, err := Configure(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	if !runtime.Enabled() || !runtime.metricsEnabled {
		t.Fatal("metrics were not enabled")
	}
	if runtime.tracingEnabled {
		t.Fatal("tracing was unexpectedly enabled")
	}
	if otel.GetMeterProvider() == originalMeter {
		t.Fatal("meter provider was not installed")
	}
	if otel.GetTracerProvider() != originalTracer {
		t.Fatal("metrics-only configuration replaced the tracer provider")
	}

	runtime.Shutdown(t.Context())

	if otel.GetMeterProvider() != originalMeter {
		t.Fatal("meter provider was not restored")
	}
}

func TestConfigureHonorsResourceAndSamplingEnvironment(t *testing.T) {
	clearTelemetryEnvironment(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "console")
	t.Setenv("OTEL_SERVICE_NAME", "operator-service")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment.name=test")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	runtime, err := Configure(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("configure telemetry: %v", err)
	}
	t.Cleanup(func() { runtime.Shutdown(t.Context()) })

	if runtime.ServiceName() != "operator-service" {
		t.Fatalf("unexpected service name %q", runtime.ServiceName())
	}
	res, err := telemetryResource(t.Context(), "testservice")
	if err != nil {
		t.Fatalf("create telemetry resource: %v", err)
	}
	deploymentEnvironment, ok := res.Set().Value("deployment.environment.name")
	if !ok || deploymentEnvironment.AsString() != "test" {
		t.Fatalf("resource attribute was not applied: %v", deploymentEnvironment)
	}
	_, span := otel.Tracer("test").Start(t.Context(), "operation")
	defer span.End()
	if span.SpanContext().IsSampled() {
		t.Fatal("always_off sampler produced a sampled span")
	}
}

func TestConfigureRejectsInvalidExplicitConfiguration(t *testing.T) {
	for _, test := range []struct {
		name     string
		exporter string
		key      string
		value    string
		code     string
	}{
		{name: "SDK disabled", key: "OTEL_SDK_DISABLED", value: "sometimes", code: "OTEL-CONFIG-SDKDISABLED"},
		{name: "exporter", key: "OTEL_TRACES_EXPORTER", value: "zipkin", code: "OTEL-CONFIG-EXPORTER"},
		{name: "protocol", key: "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", value: "json", code: "OTEL-CONFIG-EXPORTER"},
		{name: "resource", key: "OTEL_RESOURCE_ATTRIBUTES", value: "private-token", code: "OTEL-CONFIG-RESOURCE"},
		{name: "escaped resource", key: "OTEL_RESOURCE_ATTRIBUTES", value: "secret=private-token%ZZ", code: "OTEL-CONFIG-RESOURCE"},
		{name: "generic endpoint", exporter: "otlp", key: "OTEL_EXPORTER_OTLP_ENDPOINT", value: "http://private-token@%zz", code: "OTEL-CONFIG-EXPORTER"},
		{name: "trace endpoint", exporter: "otlp", key: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", value: "http://private-token@%zz", code: "OTEL-CONFIG-EXPORTER"},
		{name: "generic headers", exporter: "otlp", key: "OTEL_EXPORTER_OTLP_HEADERS", value: "Authorization=private-token%ZZ", code: "OTEL-CONFIG-EXPORTER"},
		{name: "trace headers", exporter: "otlp", key: "OTEL_EXPORTER_OTLP_TRACES_HEADERS", value: "Authorization=private-token%ZZ", code: "OTEL-CONFIG-EXPORTER"},
		{name: "sampler", key: "OTEL_TRACES_SAMPLER", value: "custom", code: "OTEL-CONFIG-SAMPLER"},
		{name: "sampler argument", key: "OTEL_TRACES_SAMPLER_ARG", value: "2", code: "OTEL-CONFIG-SAMPLER"},
		{name: "NaN sampler argument", key: "OTEL_TRACES_SAMPLER_ARG", value: "NaN", code: "OTEL-CONFIG-SAMPLER"},
		{name: "propagator", key: "OTEL_PROPAGATORS", value: "jaeger", code: "OTEL-CONFIG-PROPAGATOR"},
		{name: "batch queue", key: "OTEL_BSP_MAX_QUEUE_SIZE", value: "zero", code: "OTEL-CONFIG-BATCH"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearTelemetryEnvironment(t)
			exporter := test.exporter
			if exporter == "" {
				exporter = "console"
			}
			t.Setenv("OTEL_TRACES_EXPORTER", exporter)
			if test.key == "OTEL_SDK_DISABLED" || test.key == "OTEL_TRACES_EXPORTER" {
				t.Setenv("OTEL_TRACES_EXPORTER", "console")
			}
			if test.key == "OTEL_TRACES_SAMPLER_ARG" {
				t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
			}
			t.Setenv(test.key, test.value)

			runtime, err := Configure(t.Context(), "testservice")
			if runtime != nil {
				runtime.Shutdown(t.Context())
			}
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("expected %s error, got %v", test.code, err)
			}
			if strings.Contains(err.Error(), "private-token") {
				t.Fatalf("configuration error disclosed an operator-provided value: %v", err)
			}
		})
	}
}

func TestConfigureDoesNotLogInvalidSensitiveExporterValues(t *testing.T) {
	clearTelemetryEnvironment(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=private-token%ZZ")

	var output bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
	})

	runtime, err := Configure(t.Context(), "testservice")
	if runtime != nil {
		runtime.Shutdown(t.Context())
	}
	if err == nil || !strings.Contains(err.Error(), "OTEL-CONFIG-EXPORTER") {
		t.Fatalf("expected exporter configuration error, got %v", err)
	}
	if strings.Contains(err.Error(), "private-token") || strings.Contains(output.String(), "private-token") {
		t.Fatalf("invalid exporter configuration disclosed a sensitive value: error=%v log=%q", err, output.String())
	}
}

func TestConfigureRejectsInvalidMetricConfiguration(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value string
		code  string
	}{
		{name: "exporter", key: "OTEL_METRICS_EXPORTER", value: "statsd", code: "OTEL-CONFIG-EXPORTER"},
		{name: "protocol", key: "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", value: "json", code: "OTEL-CONFIG-EXPORTER"},
		{name: "endpoint", key: "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", value: "postgres://private-token@db", code: "OTEL-CONFIG-EXPORTER"},
		{name: "headers", key: "OTEL_EXPORTER_OTLP_METRICS_HEADERS", value: "Authorization=private-token%ZZ", code: "OTEL-CONFIG-EXPORTER"},
		{name: "interval", key: "OTEL_METRIC_EXPORT_INTERVAL", value: "immediately", code: "OTEL-CONFIG-METRICS"},
		{name: "timeout", key: "OTEL_METRIC_EXPORT_TIMEOUT", value: "0", code: "OTEL-CONFIG-METRICS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearTelemetryEnvironment(t)
			t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
			t.Setenv(test.key, test.value)

			runtime, err := Configure(t.Context(), "testservice")
			if runtime != nil {
				runtime.Shutdown(t.Context())
			}
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("expected %s error, got %v", test.code, err)
			}
			if strings.Contains(err.Error(), "private-token") {
				t.Fatalf("configuration error disclosed an operator-provided value: %v", err)
			}
		})
	}
}

func TestShutdownUsesFreshContextForProviderCleanup(t *testing.T) {
	processor := &blockingFlushProcessor{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(processor))
	originalProvider := otel.GetTracerProvider()
	originalPropagator := otel.GetTextMapPropagator()
	originalErrorHandler := otel.GetErrorHandler()
	previousProvider := nooptrace.NewTracerProvider()
	previousPropagator := propagation.NewCompositeTextMapPropagator(propagation.Baggage{})
	previousErrorHandler := &testErrorHandler{}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetErrorHandler(&testErrorHandler{})
	t.Cleanup(func() {
		otel.SetTracerProvider(originalProvider)
		otel.SetTextMapPropagator(originalPropagator)
		otel.SetErrorHandler(originalErrorHandler)
	})
	runtime := &Runtime{
		enabled:            true,
		provider:           provider,
		previousProvider:   previousProvider,
		previousPropagator: previousPropagator,
		previousError:      previousErrorHandler,
	}

	runtime.shutdown(t.Context(), time.Millisecond)

	if !processor.shutdownCalled.Load() {
		t.Fatal("provider cleanup did not run after force flush timed out")
	}
}

func TestShutdownFlushesAndCleansUpMetricProvider(t *testing.T) {
	exporter := &blockingMetricExporter{}
	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Hour))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	originalMeter := otel.GetMeterProvider()
	previousMeter := noopmetric.NewMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(originalMeter)
	})
	runtime := &Runtime{
		enabled:        true,
		metricsEnabled: true,
		metricProvider: provider,
		previousMeter:  previousMeter,
		previousError:  otel.GetErrorHandler(),
	}

	runtime.shutdown(t.Context(), time.Millisecond)

	if !exporter.flushCalled.Load() {
		t.Fatal("metric provider was not flushed")
	}
	if !exporter.shutdownCalled.Load() {
		t.Fatal("metric provider cleanup did not run after force flush timed out")
	}
	if otel.GetMeterProvider() != previousMeter {
		t.Fatal("meter provider was not restored")
	}
}

func TestConfigureRejectsInvalidInputs(t *testing.T) {
	clearTelemetryEnvironment(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "console")

	for _, test := range []struct {
		name        string
		ctx         context.Context
		serviceName string
		code        string
	}{
		{name: "context", ctx: nil, serviceName: "testservice", code: "OTEL-CONFIG-CONTEXT"},
		{name: "service name", ctx: t.Context(), serviceName: "Test Service", code: "OTEL-CONFIG-SERVICENAME"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Configure(test.ctx, test.serviceName)
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("expected %s error, got %v", test.code, err)
			}
		})
	}
}

func clearTelemetryEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range telemetryEnvironmentKeys {
		t.Setenv(key, "")
	}
}

type testErrorHandler struct{}

func (*testErrorHandler) Handle(error) {}

type blockingFlushProcessor struct {
	shutdownCalled atomic.Bool
}

func (*blockingFlushProcessor) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

func (*blockingFlushProcessor) OnEnd(sdktrace.ReadOnlySpan) {}

func (*blockingFlushProcessor) ForceFlush(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (processor *blockingFlushProcessor) Shutdown(context.Context) error {
	processor.shutdownCalled.Store(true)
	return nil
}

type blockingMetricExporter struct {
	flushCalled    atomic.Bool
	shutdownCalled atomic.Bool
}

func (*blockingMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (*blockingMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (*blockingMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	return nil
}

func (exporter *blockingMetricExporter) ForceFlush(ctx context.Context) error {
	exporter.flushCalled.Store(true)
	<-ctx.Done()
	return ctx.Err()
}

func (exporter *blockingMetricExporter) Shutdown(context.Context) error {
	exporter.shutdownCalled.Store(true)
	return nil
}
