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
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestConfigureLeavesTelemetryDisabledByDefault(t *testing.T) {
	clearTelemetryEnvironment(t)
	previousProvider := otel.GetTracerProvider()
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
	previousProvider := nooptrace.NewTracerProvider()
	previousPropagator := propagation.NewCompositeTextMapPropagator(propagation.Baggage{})
	otel.SetTracerProvider(previousProvider)
	otel.SetTextMapPropagator(previousPropagator)
	t.Cleanup(func() {
		otel.SetTracerProvider(nooptrace.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
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
	_, span := otel.Tracer("test").Start(t.Context(), "operation")
	defer span.End()
	if span.SpanContext().IsSampled() {
		t.Fatal("always_off sampler produced a sampled span")
	}
}

func TestConfigureRejectsInvalidExplicitConfiguration(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value string
		code  string
	}{
		{name: "SDK disabled", key: "OTEL_SDK_DISABLED", value: "sometimes", code: "OTEL-CONFIG-SDKDISABLED"},
		{name: "exporter", key: "OTEL_TRACES_EXPORTER", value: "zipkin", code: "OTEL-CONFIG-EXPORTER"},
		{name: "protocol", key: "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", value: "json", code: "OTEL-CONFIG-EXPORTER"},
		{name: "resource", key: "OTEL_RESOURCE_ATTRIBUTES", value: "invalid", code: "OTEL-CONFIG-RESOURCE"},
		{name: "sampler", key: "OTEL_TRACES_SAMPLER", value: "custom", code: "OTEL-CONFIG-SAMPLER"},
		{name: "sampler argument", key: "OTEL_TRACES_SAMPLER_ARG", value: "2", code: "OTEL-CONFIG-SAMPLER"},
		{name: "propagator", key: "OTEL_PROPAGATORS", value: "jaeger", code: "OTEL-CONFIG-PROPAGATOR"},
		{name: "batch queue", key: "OTEL_BSP_MAX_QUEUE_SIZE", value: "zero", code: "OTEL-CONFIG-BATCH"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearTelemetryEnvironment(t)
			t.Setenv("OTEL_TRACES_EXPORTER", "console")
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
		})
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
