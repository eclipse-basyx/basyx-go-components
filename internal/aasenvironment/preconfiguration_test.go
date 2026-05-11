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

package aasenvironment

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type mockUploadService struct {
	statusByFileName map[string]int
	calls            []string
}

func (m *mockUploadService) HandleUpload(_ context.Context, fileName string, _ string, _ *os.File) (commonmodel.ImplResponse, error) {
	m.calls = append(m.calls, fileName)
	status := m.statusByFileName[fileName]
	if status == 0 {
		status = http.StatusOK
	}
	return commonmodel.ImplResponse{Code: status, Body: map[string]any{}}, nil
}

func TestResolvePreconfigurationFiles_CollectsRecursiveSupportedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	supportedJSON := filepath.Join(tmpDir, "environment.json")
	if err := os.WriteFile(supportedJSON, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write json fixture: %v", err)
	}

	nestedDir := filepath.Join(tmpDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	supportedXML := filepath.Join(nestedDir, "environment.XML")
	if err := os.WriteFile(supportedXML, []byte("<environment/>"), 0o600); err != nil {
		t.Fatalf("failed to write xml fixture: %v", err)
	}

	unsupported := filepath.Join(tmpDir, "notes.txt")
	if err := os.WriteFile(unsupported, []byte("not an aas payload"), 0o600); err != nil {
		t.Fatalf("failed to write unsupported fixture: %v", err)
	}

	resolvedFiles, skippedCount, failedCount := resolvePreconfigurationFiles([]string{tmpDir, "", "file:" + supportedJSON})

	if failedCount != 0 {
		t.Fatalf("expected no failed sources, got %d", failedCount)
	}
	if skippedCount == 0 {
		t.Fatalf("expected at least one skipped source (empty or unsupported), got %d", skippedCount)
	}
	if len(resolvedFiles) != 2 {
		t.Fatalf("expected 2 resolved files, got %d (%v)", len(resolvedFiles), resolvedFiles)
	}
	if resolvedFiles[0] != filepath.Clean(supportedJSON) {
		t.Fatalf("unexpected first resolved file: %s", resolvedFiles[0])
	}
	if resolvedFiles[1] != filepath.Clean(supportedXML) {
		t.Fatalf("unexpected second resolved file: %s", resolvedFiles[1])
	}
}

func TestRunAASPreconfiguration_SkipsFailingImports(t *testing.T) {
	tmpDir := t.TempDir()
	okFile := filepath.Join(tmpDir, "ok.json")
	if err := os.WriteFile(okFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write ok fixture: %v", err)
	}
	failFile := filepath.Join(tmpDir, "fail.xml")
	if err := os.WriteFile(failFile, []byte("<environment/>"), 0o600); err != nil {
		t.Fatalf("failed to write fail fixture: %v", err)
	}

	uploadService := &mockUploadService{
		statusByFileName: map[string]int{
			"ok.json":  http.StatusOK,
			"fail.xml": http.StatusBadRequest,
		},
	}

	summary := RunAASPreconfiguration(context.Background(), uploadService, []string{okFile, failFile, filepath.Join(tmpDir, "missing.aasx")})

	if summary.ConfiguredSourceCount != 3 {
		t.Fatalf("expected configured source count 3, got %d", summary.ConfiguredSourceCount)
	}
	if summary.ResolvedFileCount != 2 {
		t.Fatalf("expected resolved file count 2, got %d", summary.ResolvedFileCount)
	}
	if summary.ImportedFileCount != 1 {
		t.Fatalf("expected imported file count 1, got %d", summary.ImportedFileCount)
	}
	if summary.FailedFileCount != 2 {
		t.Fatalf("expected failed file count 2, got %d", summary.FailedFileCount)
	}
	if len(uploadService.calls) != 2 {
		t.Fatalf("expected upload service to be called twice, got %d", len(uploadService.calls))
	}
}
