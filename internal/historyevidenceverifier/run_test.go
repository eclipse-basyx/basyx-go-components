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

package historyevidenceverifier

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeepsDiagnosticJSONOnStderr(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := []byte(`logging:
  format: json
  level: info
postgres:
  host: 127.0.0.1
  port: 1
  connectTimeoutSeconds: 1
`)
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		context.TODO(),
		[]string{"-config", configPath, "-table", "aas_history", "-from", "1", "-to", "1"},
		&stdout,
		&stderr,
	)

	if exitCode != exitFailure {
		t.Fatalf("expected failure exit code, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected stdout to contain only command results, got %q", stdout.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected configuration and error records, got %q", stderr.String())
	}
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("stderr record is not JSON: %q: %v", line, err)
		}
		if record["service.name"] != "historyevidenceverifier" {
			t.Fatalf("unexpected service name in %#v", record)
		}
	}
	if !strings.Contains(lines[len(lines)-1], `"error.code":"HISTORYVERIFY-RUN-EXECUTE"`) {
		t.Fatalf("expected coded operation error, got %q", lines[len(lines)-1])
	}
}

func TestWriteJSONOutputUsesOnlyProvidedResultWriter(t *testing.T) {
	var stdout bytes.Buffer
	if err := writeJSONOutput(map[string]any{"valid": true}, "", &stdout); err != nil {
		t.Fatalf("write JSON output: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not a JSON result: %v", err)
	}
	if result["valid"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
}
