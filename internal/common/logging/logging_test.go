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

package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestConfigureJSONProducesOneStructuredObject(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})

	slog.Info("configuration loaded", "port", 8080, "error", errors.New("example"))

	lines := nonEmptyLines(output.String())
	if len(lines) != 1 {
		t.Fatalf("expected one JSON object, got %d lines: %q", len(lines), output.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("unmarshal log record: %v", err)
	}
	for _, key := range []string{"time", "level", "msg", "service.name", "port", "error"} {
		if _, ok := record[key]; !ok {
			t.Errorf("expected key %q in record %#v", key, record)
		}
	}
	if record["level"] != "INFO" || record["msg"] != "configuration loaded" || record["service.name"] != "testservice" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record["port"] != float64(8080) || record["error"] != "example" {
		t.Fatalf("event attributes did not serialize correctly: %#v", record)
	}
}

func TestConfigureTextIncludesEnvelopeAndServiceName(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatText, Level: LevelInfo})

	slog.Warn("fallback active", "attempt", 2)

	line := output.String()
	for _, expected := range []string{"time=", "level=WARN", `msg="fallback active"`, "service.name=testservice", "attempt=2"} {
		if !strings.Contains(line, expected) {
			t.Errorf("expected %q in %q", expected, line)
		}
	}
}

func TestConfigureFiltersByMinimumLevel(t *testing.T) {
	for _, test := range []struct {
		level       string
		expected    []string
		notExpected []string
	}{
		{level: LevelDebug, expected: []string{"debug", "info", "warn", "error"}},
		{level: LevelInfo, expected: []string{"info", "warn", "error"}, notExpected: []string{"debug"}},
		{level: LevelWarn, expected: []string{"warn", "error"}, notExpected: []string{"debug", "info"}},
		{level: LevelError, expected: []string{"error"}, notExpected: []string{"debug", "info", "warn"}},
	} {
		t.Run(test.level, func(t *testing.T) {
			output := configureForTest(t, Config{Format: FormatJSON, Level: test.level})
			slog.Debug("debug")
			slog.Info("info")
			slog.Warn("warn")
			slog.Error("error")

			for _, expected := range test.expected {
				if !strings.Contains(output.String(), `"msg":"`+expected+`"`) {
					t.Errorf("expected %q event in %q", expected, output.String())
				}
			}
			for _, notExpected := range test.notExpected {
				if strings.Contains(output.String(), `"msg":"`+notExpected+`"`) {
					t.Errorf("did not expect %q event in %q", notExpected, output.String())
				}
			}
		})
	}
}

func TestConfigureBridgesStandardLogAtInfo(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})

	log.Print("dependency message")

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("unmarshal bridged record: %v", err)
	}
	if record["level"] != "INFO" || record["msg"] != "dependency message" || record["service.name"] != "testservice" {
		t.Fatalf("unexpected bridged record: %#v", record)
	}
}

func TestConfigureEnrichesContextualLogsWithRequestMetadata(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	ctx := contextWithRequestMetadata(t.Context(), requestMetadata{
		requestID:     "request-1",
		correlationID: "correlation-1",
	})

	slog.InfoContext(ctx, "request operation completed")

	record := decodeSingleRecord(t, output)
	if record["request.id"] != "request-1" {
		t.Fatalf("unexpected request.id: %#v", record)
	}
	if record["correlation.id"] != "correlation-1" {
		t.Fatalf("unexpected correlation.id: %#v", record)
	}
}

func TestConfigureEnrichesContextualLogsWithTraceMetadata(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("parse trace ID: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("parse span ID: %v", err)
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), spanContext)

	slog.InfoContext(ctx, "traced operation completed")

	record := decodeSingleRecord(t, output)
	if record["trace_id"] != traceID.String() {
		t.Fatalf("unexpected trace_id: %#v", record)
	}
	if record["span_id"] != spanID.String() {
		t.Fatalf("unexpected span_id: %#v", record)
	}
	if record["trace_flags"] != "01" {
		t.Fatalf("unexpected trace_flags: %#v", record)
	}
}

func TestConfigureIncludesUnsampledTraceMetadata(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatText, Level: LevelInfo})
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))

	slog.InfoContext(ctx, "unsampled operation")

	line := output.String()
	for _, expected := range []string{
		"trace_id=" + traceID.String(),
		"span_id=" + spanID.String(),
		"trace_flags=00",
	} {
		if !strings.Contains(line, expected) {
			t.Errorf("expected %q in %q", expected, line)
		}
	}
}

func TestConfigureDoesNotAddRequestMetadataToBackgroundLogs(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})

	slog.Info("background operation completed")

	record := decodeSingleRecord(t, output)
	if _, ok := record["request.id"]; ok {
		t.Fatalf("background record contains request.id: %#v", record)
	}
	if _, ok := record["correlation.id"]; ok {
		t.Fatalf("background record contains correlation.id: %#v", record)
	}
	for _, key := range []string{"trace_id", "span_id", "trace_flags"} {
		if _, ok := record[key]; ok {
			t.Fatalf("background record contains %s: %#v", key, record)
		}
	}
}

func TestContextHandlerPreservesAttributesAndGroups(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelInfo})
	ctx := contextWithRequestMetadata(t.Context(), requestMetadata{
		requestID:     "request-1",
		correlationID: "correlation-1",
	})
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))
	logger := slog.Default().With("component", "test").WithGroup("details")

	logger.InfoContext(ctx, "grouped event", "attempt", 2)

	record := decodeSingleRecord(t, output)
	if record["request.id"] != "request-1" || record["correlation.id"] != "correlation-1" {
		t.Fatalf("request metadata is not at the record root: %#v", record)
	}
	if record["trace_id"] != traceID.String() || record["span_id"] != spanID.String() {
		t.Fatalf("trace metadata is not at the record root: %#v", record)
	}
	if record["component"] != "test" {
		t.Fatalf("preformatted attribute missing: %#v", record)
	}
	details, ok := record["details"].(map[string]any)
	if !ok || details["attempt"] != float64(2) {
		t.Fatalf("grouped event attribute missing: %#v", record)
	}
}

func TestRequestMetadataContextAccessors(t *testing.T) {
	ctx := contextWithRequestMetadata(t.Context(), requestMetadata{
		requestID:     "request-1",
		correlationID: "correlation-1",
	})
	if RequestIDFromContext(ctx) != "request-1" {
		t.Fatalf("unexpected request ID %q", RequestIDFromContext(ctx))
	}
	if CorrelationIDFromContext(ctx) != "correlation-1" {
		t.Fatalf("unexpected correlation ID %q", CorrelationIDFromContext(ctx))
	}
}

func TestStandardLogBridgeHonorsInfoThreshold(t *testing.T) {
	output := configureForTest(t, Config{Format: FormatJSON, Level: LevelWarn})

	log.Print("dependency message")

	if output.Len() != 0 {
		t.Fatalf("expected bridged INFO record to be filtered, got %q", output.String())
	}
}

func TestConfigureRejectsInvalidInputs(t *testing.T) {
	var nilBuffer *bytes.Buffer
	for _, test := range []struct {
		name        string
		cfg         Config
		serviceName string
		output      any
		code        string
	}{
		{name: "format", cfg: Config{Format: "yaml", Level: LevelInfo}, serviceName: "service", output: &bytes.Buffer{}, code: "CONFIG-LOGGING-FORMAT"},
		{name: "empty format", cfg: Config{Format: "", Level: LevelInfo}, serviceName: "service", output: &bytes.Buffer{}, code: "CONFIG-LOGGING-FORMAT"},
		{name: "level", cfg: Config{Format: FormatText, Level: "trace"}, serviceName: "service", output: &bytes.Buffer{}, code: "CONFIG-LOGGING-LEVEL"},
		{name: "empty level", cfg: Config{Format: FormatText, Level: ""}, serviceName: "service", output: &bytes.Buffer{}, code: "CONFIG-LOGGING-LEVEL"},
		{name: "service name", cfg: Config{Format: FormatText, Level: LevelInfo}, serviceName: " ", output: &bytes.Buffer{}, code: "LOGGING-CONFIG-SERVICENAME"},
		{name: "uppercase service name", cfg: Config{Format: FormatText, Level: LevelInfo}, serviceName: "TestService", output: &bytes.Buffer{}, code: "LOGGING-CONFIG-SERVICENAME"},
		{name: "output", cfg: Config{Format: FormatText, Level: LevelInfo}, serviceName: "service", output: nil, code: "LOGGING-CONFIG-OUTPUT"},
		{name: "typed nil output", cfg: Config{Format: FormatText, Level: LevelInfo}, serviceName: "service", output: nilBuffer, code: "LOGGING-CONFIG-OUTPUT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer, _ := test.output.(*bytes.Buffer)
			_, err := Configure(test.cfg, test.serviceName, writer)
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("expected %s error, got %v", test.code, err)
			}
		})
	}
}

func TestNormalizeCanonicalizesCaseAndWhitespace(t *testing.T) {
	cfg, err := Normalize(Config{Format: " JSON ", Level: " WaRn "})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.Format != FormatJSON || cfg.Level != LevelWarn {
		t.Fatalf("unexpected normalized config: %#v", cfg)
	}
}

func configureForTest(t *testing.T, cfg Config) *bytes.Buffer {
	t.Helper()
	previousLogger := slog.Default()
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})
	var output bytes.Buffer
	if _, err := Configure(cfg, "testservice", &output); err != nil {
		t.Fatalf("configure: %v", err)
	}
	return &output
}

func nonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func decodeSingleRecord(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()
	lines := nonEmptyLines(output.String())
	if len(lines) != 1 {
		t.Fatalf("expected one JSON object, got %d lines: %q", len(lines), output.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("unmarshal log record: %v", err)
	}
	return record
}
