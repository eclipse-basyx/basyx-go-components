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

// Package telemetry configures optional process-wide OpenTelemetry signals.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/textproto"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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
	"OTEL_METRICS_EXPORTER",
	"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL",
	"OTEL_EXPORTER_OTLP_METRICS_HEADERS",
	"OTEL_EXPORTER_OTLP_METRICS_COMPRESSION",
	"OTEL_EXPORTER_OTLP_METRICS_TIMEOUT",
	"OTEL_METRIC_EXPORT_INTERVAL",
	"OTEL_METRIC_EXPORT_TIMEOUT",
}

var activeRuntime atomic.Pointer[Runtime]

// Runtime owns the global OpenTelemetry state installed by Configure.
type Runtime struct {
	enabled            bool
	tracingEnabled     bool
	metricsEnabled     bool
	serviceName        string
	provider           *sdktrace.TracerProvider
	metricProvider     *sdkmetric.MeterProvider
	previousProvider   trace.TracerProvider
	previousMeter      otelmetric.MeterProvider
	previousPropagator propagation.TextMapPropagator
	previousError      otel.ErrorHandler
	databasePools      databasePoolRegistry
	shutdownOnce       sync.Once
}

type telemetryConfiguration struct {
	enabled        bool
	traceEnabled   bool
	metricsEnabled bool
	resource       *resource.Resource
	sampler        sdktrace.Sampler
	propagator     propagation.TextMapPropagator
}

type exporterEnvironment struct {
	endpoint    string
	headers     string
	protocol    string
	compression string
	timeout     string
}

// Configure initializes optional telemetry from standard OpenTelemetry
// environment variables.
func Configure(ctx context.Context, serviceName string) (*Runtime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("OTEL-CONFIG-CONTEXT context must not be nil")
	}
	if !validServiceName(serviceName) {
		return nil, fmt.Errorf("OTEL-CONFIG-SERVICENAME invalid service name %q", serviceName)
	}

	cfg, err := resolveTelemetryConfiguration(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	if !cfg.enabled {
		return &Runtime{serviceName: serviceName}, nil
	}

	runtime := &Runtime{
		enabled:            true,
		tracingEnabled:     cfg.traceEnabled,
		metricsEnabled:     cfg.metricsEnabled,
		serviceName:        resourceServiceName(cfg.resource, serviceName),
		previousProvider:   otel.GetTracerProvider(),
		previousMeter:      otel.GetMeterProvider(),
		previousPropagator: otel.GetTextMapPropagator(),
		previousError:      otel.GetErrorHandler(),
	}
	if err = runtime.initializeProviders(ctx, cfg); err != nil {
		return nil, err
	}
	runtime.install(cfg.propagator)
	return runtime, nil
}

func resolveTelemetryConfiguration(ctx context.Context, serviceName string) (*telemetryConfiguration, error) {
	disabled, err := sdkDisabled()
	if err != nil {
		return nil, err
	}
	if disabled {
		return &telemetryConfiguration{}, nil
	}

	_, traceEnabled, err := configuredExporter("OTEL_TRACES_EXPORTER")
	if err != nil {
		return nil, err
	}
	_, metricsEnabled, err := configuredExporter("OTEL_METRICS_EXPORTER")
	if err != nil {
		return nil, err
	}
	cfg := &telemetryConfiguration{
		enabled:        traceEnabled || metricsEnabled,
		traceEnabled:   traceEnabled,
		metricsEnabled: metricsEnabled,
	}
	if !cfg.enabled {
		return cfg, nil
	}
	if err = validateExporterEnvironment(traceEnabled, metricsEnabled); err != nil {
		return nil, err
	}
	if traceEnabled {
		cfg.sampler, cfg.propagator, err = resolveTraceConfiguration()
		if err != nil {
			return nil, err
		}
	}
	if metricsEnabled {
		if err = validateMetricEnvironment(); err != nil {
			return nil, err
		}
	}
	cfg.resource, err = telemetryResource(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func resolveTraceConfiguration() (sdktrace.Sampler, propagation.TextMapPropagator, error) {
	sampler, err := samplerFromEnvironment()
	if err != nil {
		return nil, nil, err
	}
	if err = validateBatchEnvironment(); err != nil {
		return nil, nil, err
	}
	propagator, err := propagatorFromEnvironment()
	if err != nil {
		return nil, nil, err
	}
	return sampler, propagator, nil
}

func validateMetricEnvironment() error {
	for _, key := range []string{"OTEL_METRIC_EXPORT_INTERVAL", "OTEL_METRIC_EXPORT_TIMEOUT"} {
		if err := validatePositiveIntegerEnvironment(key, "OTEL-CONFIG-METRICS"); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *Runtime) initializeProviders(ctx context.Context, cfg *telemetryConfiguration) error {
	if cfg.traceEnabled {
		if err := runtime.initializeTraceProvider(ctx, cfg); err != nil {
			return err
		}
	}
	if cfg.metricsEnabled {
		if err := runtime.initializeMetricProvider(ctx, cfg); err != nil {
			runtime.shutdownProviders(ctx)
			return err
		}
	}
	return nil
}

func (runtime *Runtime) initializeTraceProvider(ctx context.Context, cfg *telemetryConfiguration) error {
	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return fmt.Errorf("OTEL-CONFIG-EXPORTER invalid trace exporter configuration (%s)", telemetryErrorType(err))
	}
	restoreSamplerEnvironment := unsetEmptyEnvironment("OTEL_TRACES_SAMPLER")
	defer restoreSamplerEnvironment()
	runtime.provider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(cfg.resource),
		sdktrace.WithSampler(cfg.sampler),
		sdktrace.WithBatcher(exporter),
	)
	return nil
}

func (runtime *Runtime) initializeMetricProvider(ctx context.Context, cfg *telemetryConfiguration) error {
	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return fmt.Errorf("OTEL-CONFIG-EXPORTER invalid metric exporter configuration (%s)", telemetryErrorType(err))
	}
	runtime.metricProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(cfg.resource),
		sdkmetric.WithReader(reader),
	)
	return runtime.databasePools.initialize(runtime.metricProvider.Meter(instrumentationName))
}

func (runtime *Runtime) install(propagator propagation.TextMapPropagator) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(handleRuntimeError))
	if runtime.tracingEnabled {
		otel.SetTextMapPropagator(propagator)
		otel.SetTracerProvider(runtime.provider)
	}
	if runtime.metricsEnabled {
		otel.SetMeterProvider(runtime.metricProvider)
	}
	activeRuntime.Store(runtime)
}

// Enabled reports whether this runtime installed any active telemetry signal.
func (runtime *Runtime) Enabled() bool {
	return runtime != nil && runtime.enabled
}

// ServiceName returns the effective telemetry resource service name.
func (runtime *Runtime) ServiceName() string {
	if runtime == nil {
		return ""
	}
	return runtime.serviceName
}

// Shutdown flushes pending telemetry, shuts down the providers, and restores the
// process-wide OpenTelemetry state that Configure replaced.
func (runtime *Runtime) Shutdown(ctx context.Context) {
	runtime.shutdown(ctx, shutdownTimeout)
}

func (runtime *Runtime) shutdown(ctx context.Context, timeout time.Duration) {
	if runtime == nil || !runtime.enabled {
		return
	}
	runtime.shutdownOnce.Do(func() {
		traceActive := runtime.tracingEnabled || runtime.provider != nil
		metricsActive := runtime.metricsEnabled || runtime.metricProvider != nil
		shutdownParent := context.TODO()
		if ctx != nil {
			shutdownParent = context.WithoutCancel(ctx)
		}

		flushCtx, cancelFlush := context.WithTimeout(shutdownParent, timeout)
		runtime.flushProviders(flushCtx, traceActive, metricsActive)
		cancelFlush()

		runtime.databasePools.unregister()
		shutdownCtx, cancelShutdown := context.WithTimeout(shutdownParent, timeout)
		runtime.shutdownActiveProviders(shutdownCtx, traceActive, metricsActive)
		cancelShutdown()

		if traceActive {
			otel.SetTracerProvider(runtime.previousProvider)
			otel.SetTextMapPropagator(runtime.previousPropagator)
		}
		if metricsActive {
			otel.SetMeterProvider(runtime.previousMeter)
		}
		otel.SetErrorHandler(runtime.previousError)
		activeRuntime.CompareAndSwap(runtime, nil)
	})
}

func (runtime *Runtime) flushProviders(ctx context.Context, traceActive bool, metricsActive bool) {
	var waitGroup sync.WaitGroup
	if metricsActive {
		waitGroup.Go(func() {
			if err := runtime.metricProvider.ForceFlush(ctx); err != nil {
				logRuntimeWarning("OpenTelemetry metric flush failed", "OTEL-RUNTIME-METRICFLUSH", err)
			}
		})
	}
	if traceActive {
		waitGroup.Go(func() {
			if err := runtime.provider.ForceFlush(ctx); err != nil {
				logRuntimeWarning("OpenTelemetry trace flush failed", "OTEL-RUNTIME-TRACEFLUSH", err)
			}
		})
	}
	waitGroup.Wait()
}

func (runtime *Runtime) shutdownActiveProviders(ctx context.Context, traceActive bool, metricsActive bool) {
	var waitGroup sync.WaitGroup
	if metricsActive {
		waitGroup.Go(func() {
			if err := runtime.metricProvider.Shutdown(ctx); err != nil {
				logRuntimeWarning("OpenTelemetry metric shutdown failed", "OTEL-RUNTIME-METRICSHUTDOWN", err)
			}
		})
	}
	if traceActive {
		waitGroup.Go(func() {
			if err := runtime.provider.Shutdown(ctx); err != nil {
				logRuntimeWarning("OpenTelemetry trace shutdown failed", "OTEL-RUNTIME-TRACESHUTDOWN", err)
			}
		})
	}
	waitGroup.Wait()
}

func (runtime *Runtime) shutdownProviders(ctx context.Context) {
	if runtime.metricProvider != nil {
		_ = runtime.metricProvider.Shutdown(ctx)
	}
	if runtime.provider != nil {
		_ = runtime.provider.Shutdown(ctx)
	}
}

func configuredExporter(key string) (string, bool, error) {
	name := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch name {
	case "", "none":
		return name, false, nil
	case "otlp", "console":
		return name, true, nil
	default:
		return "", false, fmt.Errorf("OTEL-CONFIG-EXPORTER unsupported %s %q", key, name)
	}
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

func validateExporterEnvironment(traceEnabled bool, metricEnabled bool) error {
	for _, environment := range exporterEnvironments(traceEnabled, metricEnabled) {
		if err := validateExporterEnvironmentValues(environment); err != nil {
			return err
		}
	}
	return nil
}

func exporterEnvironments(traceEnabled bool, metricEnabled bool) []exporterEnvironment {
	environments := make([]exporterEnvironment, 0, 3)
	if traceEnabled || metricEnabled {
		environments = append(environments, newExporterEnvironment(""))
	}
	if traceEnabled {
		environments = append(environments, newExporterEnvironment("_TRACES"))
	}
	if metricEnabled {
		environments = append(environments, newExporterEnvironment("_METRICS"))
	}
	return environments
}

func newExporterEnvironment(signal string) exporterEnvironment {
	const prefix = "OTEL_EXPORTER_OTLP"
	return exporterEnvironment{
		endpoint:    prefix + signal + "_ENDPOINT",
		headers:     prefix + signal + "_HEADERS",
		protocol:    prefix + signal + "_PROTOCOL",
		compression: prefix + signal + "_COMPRESSION",
		timeout:     prefix + signal + "_TIMEOUT",
	}
}

func validateExporterEnvironmentValues(environment exporterEnvironment) error {
	if err := validateEndpointEnvironment(environment.endpoint); err != nil {
		return err
	}
	if err := validateHeaderEnvironment(environment.headers); err != nil {
		return err
	}
	if err := validateProtocolEnvironment(environment.protocol); err != nil {
		return err
	}
	if err := validateCompressionEnvironment(environment.compression); err != nil {
		return err
	}
	return validatePositiveIntegerEnvironment(environment.timeout, "OTEL-CONFIG-EXPORTER")
}

func validateProtocolEnvironment(key string) error {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value != "" && value != "grpc" && value != "http/protobuf" {
		return fmt.Errorf("OTEL-CONFIG-EXPORTER unsupported %s %q", key, value)
	}
	return nil
}

func validateCompressionEnvironment(key string) error {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value != "" && value != "gzip" && value != "none" {
		return fmt.Errorf("OTEL-CONFIG-EXPORTER unsupported %s %q", key, value)
	}
	return nil
}

func validateEndpointEnvironment(key string) error {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return fmt.Errorf("OTEL-CONFIG-EXPORTER invalid %s", key)
	}
	return nil
}

func validateHeaderEnvironment(key string) error {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	for _, header := range strings.Split(value, ",") {
		name, encodedValue, found := strings.Cut(header, "=")
		if !found || textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name)) == "" {
			return fmt.Errorf("OTEL-CONFIG-EXPORTER invalid %s", key)
		}
		decodedValue, err := url.PathUnescape(encodedValue)
		if err != nil || !validHeaderValue(decodedValue) {
			return fmt.Errorf("OTEL-CONFIG-EXPORTER invalid %s", key)
		}
	}
	return nil
}

func validHeaderValue(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '\t' || character >= ' ' && character != 0x7f {
			continue
		}
		return false
	}
	return true
}

func telemetryResource(ctx context.Context, serviceName string) (*resource.Resource, error) {
	if err := validateResourceEnvironment(); err != nil {
		return nil, err
	}
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

func validateResourceEnvironment() error {
	value := strings.TrimSpace(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))
	if value == "" {
		return nil
	}
	for _, item := range strings.Split(value, ",") {
		key, encodedValue, found := strings.Cut(item, "=")
		if !found || strings.TrimSpace(key) == "" {
			return fmt.Errorf("OTEL-CONFIG-RESOURCE invalid OTEL_RESOURCE_ATTRIBUTES")
		}
		if _, err := url.PathUnescape(strings.TrimSpace(encodedValue)); err != nil {
			return fmt.Errorf("OTEL-CONFIG-RESOURCE invalid OTEL_RESOURCE_ATTRIBUTES")
		}
	}
	return nil
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
	if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
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
