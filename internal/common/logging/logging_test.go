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
