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

// Package telemetry configures optional process-wide OpenTelemetry tracing.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	shutdownTimeout = 5 * time.Second
)

var telemetryEnvironmentKeys = []string{
	"OTEL_SDK_DISABLED",
	"OTEL_TRACES_EXPORTER",
	"OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
	"OTEL_EXPORTER_OTLP_PROTOCOL",
	"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
	"OTEL_EXPORTER_OTLP_HEADERS",
	"OTEL_EXPORTER_OTLP_TRACES_HEADERS",
	"OTEL_EXPORTER_OTLP_COMPRESSION",
	"OTEL_EXPORTER_OTLP_TRACES_COMPRESSION",
	"OTEL_EXPORTER_OTLP_TIMEOUT",
	"OTEL_EXPORTER_OTLP_TRACES_TIMEOUT",
	"OTEL_SERVICE_NAME",
	"OTEL_RESOURCE_ATTRIBUTES",
	"OTEL_TRACES_SAMPLER",
	"OTEL_TRACES_SAMPLER_ARG",
	"OTEL_BSP_SCHEDULE_DELAY",
	"OTEL_BSP_EXPORT_TIMEOUT",
	"OTEL_BSP_MAX_QUEUE_SIZE",
	"OTEL_BSP_MAX_EXPORT_BATCH_SIZE",
	"OTEL_PROPAGATORS",
}

var activeRuntime atomic.Pointer[Runtime]

// Runtime owns the global tracing state installed by Configure.
type Runtime struct {
	enabled            bool
	serviceName        string
	provider           *sdktrace.TracerProvider
	previousProvider   trace.TracerProvider
	previousPropagator propagation.TextMapPropagator
	previousError      otel.ErrorHandler
	shutdownOnce       sync.Once
}

// Configure initializes optional tracing from standard OpenTelemetry
// environment variables.
func Configure(ctx context.Context, serviceName string) (*Runtime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("OTEL-CONFIG-CONTEXT context must not be nil")
	}
	if !validServiceName(serviceName) {
		return nil, fmt.Errorf("OTEL-CONFIG-SERVICENAME invalid service name %q", serviceName)
	}

	disabled, err := sdkDisabled()
	if err != nil {
		return nil, err
	}
	exporterName := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER")))
	if disabled || exporterName == "" || exporterName == "none" {
		return &Runtime{serviceName: serviceName}, nil
	}
	if exporterName != "otlp" && exporterName != "console" {
		return nil, fmt.Errorf("OTEL-CONFIG-EXPORTER unsupported OTEL_TRACES_EXPORTER %q", exporterName)
	}
	if err := validateExporterEnvironment(); err != nil {
		return nil, err
	}
	sampler, err := samplerFromEnvironment()
	if err != nil {
		return nil, err
	}
	if err := validateBatchEnvironment(); err != nil {
		return nil, err
	}
	propagator, err := propagatorFromEnvironment()
	if err != nil {
		return nil, err
	}
	res, err := telemetryResource(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("OTEL-CONFIG-EXPORTER invalid exporter configuration (%s)", telemetryErrorType(err))
	}

	runtime := &Runtime{
		enabled:            true,
		serviceName:        resourceServiceName(res, serviceName),
		previousProvider:   otel.GetTracerProvider(),
		previousPropagator: otel.GetTextMapPropagator(),
		previousError:      otel.GetErrorHandler(),
	}
	restoreSamplerEnvironment := unsetEmptyEnvironment("OTEL_TRACES_SAMPLER")
	defer restoreSamplerEnvironment()
	runtime.provider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(handleRuntimeError))
	otel.SetTextMapPropagator(propagator)
	otel.SetTracerProvider(runtime.provider)
	activeRuntime.Store(runtime)
	return runtime, nil
}

// Enabled reports whether this runtime installed active tracing.
func (runtime *Runtime) Enabled() bool {
	return runtime != nil && runtime.enabled
}

// ServiceName returns the effective trace resource service name.
func (runtime *Runtime) ServiceName() string {
	if runtime == nil {
		return ""
	}
	return runtime.serviceName
}

// Shutdown flushes pending spans, shuts down the provider, and restores the
// process-wide OpenTelemetry state that Configure replaced.
func (runtime *Runtime) Shutdown(ctx context.Context) {
	if runtime == nil || !runtime.enabled {
		return
	}
	runtime.shutdownOnce.Do(func() {
		shutdownParent := context.TODO()
		if ctx != nil {
			shutdownParent = context.WithoutCancel(ctx)
		}
		shutdownCtx, cancel := context.WithTimeout(shutdownParent, shutdownTimeout)
		defer cancel()

		if err := runtime.provider.ForceFlush(shutdownCtx); err != nil {
			logRuntimeWarning("OpenTelemetry flush failed", "OTEL-RUNTIME-FLUSH", err)
		}
		if err := runtime.provider.Shutdown(shutdownCtx); err != nil {
			logRuntimeWarning("OpenTelemetry shutdown failed", "OTEL-RUNTIME-SHUTDOWN", err)
		}
		otel.SetTracerProvider(runtime.previousProvider)
		otel.SetTextMapPropagator(runtime.previousPropagator)
		otel.SetErrorHandler(runtime.previousError)
		activeRuntime.CompareAndSwap(runtime, nil)
	})
}

func sdkDisabled() (bool, error) {
	value, explicit := os.LookupEnv("OTEL_SDK_DISABLED")
	value = strings.TrimSpace(value)
	if !explicit || value == "" {
		return false, nil
	}
	disabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("OTEL-CONFIG-SDKDISABLED invalid OTEL_SDK_DISABLED %q", value)
	}
	return disabled, nil
}

func validateExporterEnvironment() error {
	for _, key := range []string{"OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value != "" && value != "grpc" && value != "http/protobuf" {
			return fmt.Errorf("OTEL-CONFIG-EXPORTER unsupported %s %q", key, value)
		}
	}
	for _, key := range []string{"OTEL_EXPORTER_OTLP_COMPRESSION", "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value != "" && value != "gzip" && value != "none" {
			return fmt.Errorf("OTEL-CONFIG-EXPORTER unsupported %s %q", key, value)
		}
	}
	for _, key := range []string{"OTEL_EXPORTER_OTLP_TIMEOUT", "OTEL_EXPORTER_OTLP_TRACES_TIMEOUT"} {
		if err := validatePositiveIntegerEnvironment(key, "OTEL-CONFIG-EXPORTER"); err != nil {
			return err
		}
	}
	return nil
}

func telemetryResource(ctx context.Context, serviceName string) (*resource.Resource, error) {
	res, err := resource.New(
		ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("OTEL-CONFIG-RESOURCE invalid resource configuration (%s)", telemetryErrorType(err))
	}
	return res, nil
}

func resourceServiceName(res *resource.Resource, fallback string) string {
	value, ok := res.Set().Value(semconv.ServiceNameKey)
	if !ok {
		return fallback
	}
	return value.AsString()
}

func samplerFromEnvironment() (sdktrace.Sampler, error) {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER")))
	argument := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
	switch name {
	case "", "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case "always_on":
		return sdktrace.AlwaysSample(), nil
	case "always_off":
		return sdktrace.NeverSample(), nil
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case "traceidratio", "parentbased_traceidratio":
		ratio, err := traceRatio(argument)
		if err != nil {
			return nil, err
		}
		sampler := sdktrace.TraceIDRatioBased(ratio)
		if name == "parentbased_traceidratio" {
			return sdktrace.ParentBased(sampler), nil
		}
		return sampler, nil
	default:
		return nil, fmt.Errorf("OTEL-CONFIG-SAMPLER unsupported OTEL_TRACES_SAMPLER %q", name)
	}
}

func traceRatio(argument string) (float64, error) {
	if argument == "" {
		return 1, nil
	}
	ratio, err := strconv.ParseFloat(argument, 64)
	if err != nil || ratio < 0 || ratio > 1 {
		return 0, fmt.Errorf("OTEL-CONFIG-SAMPLER invalid OTEL_TRACES_SAMPLER_ARG %q", argument)
	}
	return ratio, nil
}

func validateBatchEnvironment() error {
	for _, key := range []string{
		"OTEL_BSP_SCHEDULE_DELAY",
		"OTEL_BSP_EXPORT_TIMEOUT",
		"OTEL_BSP_MAX_QUEUE_SIZE",
		"OTEL_BSP_MAX_EXPORT_BATCH_SIZE",
	} {
		if err := validatePositiveIntegerEnvironment(key, "OTEL-CONFIG-BATCH"); err != nil {
			return err
		}
	}

	queueSize, queueSet := positiveIntegerEnvironment("OTEL_BSP_MAX_QUEUE_SIZE")
	batchSize, batchSet := positiveIntegerEnvironment("OTEL_BSP_MAX_EXPORT_BATCH_SIZE")
	if queueSet && batchSet && batchSize > queueSize {
		return fmt.Errorf("OTEL-CONFIG-BATCH OTEL_BSP_MAX_EXPORT_BATCH_SIZE must not exceed OTEL_BSP_MAX_QUEUE_SIZE")
	}
	return nil
}

func validatePositiveIntegerEnvironment(key string, code string) error {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("%s invalid %s %q", code, key, value)
	}
	return nil
}

func positiveIntegerEnvironment(key string) (int, bool) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil && parsed > 0
}

func propagatorFromEnvironment() (propagation.TextMapPropagator, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_PROPAGATORS")))
	if value == "" {
		return propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		), nil
	}
	if value == "none" {
		return propagation.NewCompositeTextMapPropagator(), nil
	}

	var propagators []propagation.TextMapPropagator
	seen := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if seen[name] {
			continue
		}
		seen[name] = true
		switch name {
		case "tracecontext":
			propagators = append(propagators, propagation.TraceContext{})
		case "baggage":
			propagators = append(propagators, propagation.Baggage{})
		default:
			return nil, fmt.Errorf("OTEL-CONFIG-PROPAGATOR unsupported OTEL_PROPAGATORS value %q", name)
		}
	}
	return propagation.NewCompositeTextMapPropagator(propagators...), nil
}

func validServiceName(serviceName string) bool {
	if serviceName == "" || strings.TrimSpace(serviceName) != serviceName {
		return false
	}
	for index, character := range serviceName {
		isLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && character != '-' {
			return false
		}
		if index == 0 && character == '-' {
			return false
		}
	}
	return !strings.HasSuffix(serviceName, "-")
}

func handleRuntimeError(err error) {
	logRuntimeWarning("OpenTelemetry export failed", "OTEL-RUNTIME-EXPORT", err)
}

func logRuntimeWarning(message string, code string, err error) {
	slog.Warn(
		message,
		"error.code", code,
		"error.type", telemetryErrorType(err),
	)
}

func telemetryErrorType(err error) string {
	if err == nil {
		return "<nil>"
	}
	return reflect.TypeOf(err).String()
}

func unsetEmptyEnvironment(key string) func() {
	value, exists := os.LookupEnv(key)
	if !exists || value != "" {
		return func() {}
	}
	_ = os.Unsetenv(key)
	return func() {
		_ = os.Setenv(key, value)
	}
}
