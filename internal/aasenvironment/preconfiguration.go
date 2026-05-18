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
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	preconfigurationComponent = "AASENV"
	preconfigurationOperation = "Preconfiguration"
)

var supportedAASPreconfigurationExtensions = map[string]struct{}{
	".aasx": {},
	".json": {},
	".xml":  {},
}

// PreconfigurationSummary describes the startup preconfiguration outcome.
type PreconfigurationSummary struct {
	ConfiguredSourceCount int
	ResolvedFileCount     int
	ImportedFileCount     int
	FailedFileCount       int
	SkippedFileCount      int
}

// RunAASPreconfiguration resolves configured files and folders and imports
// them into the AAS environment.
//
// Parameters:
//   - ctx: Optional context used for cancellation.
//   - uploadService: Service used to upload/import resolved files.
//   - configuredSources: List of file or directory paths to import.
//
// Returns:
//   - PreconfigurationSummary containing import statistics such as resolved,
//     imported, skipped, and failed files.
func RunAASPreconfiguration(ctx context.Context, uploadService UploadService, configuredSources []string) PreconfigurationSummary {
	summary := PreconfigurationSummary{
		ConfiguredSourceCount: len(configuredSources),
	}

	if uploadService == nil {
		summary.FailedFileCount = len(configuredSources)
		log.Printf("%s-%s-NILUPLOADSERVICE upload service is required", preconfigurationComponent, preconfigurationOperation)
		return summary
	}

	resolvedFiles, skippedCount, failedCount := resolvePreconfigurationFiles(configuredSources)
	summary.ResolvedFileCount = len(resolvedFiles)
	summary.SkippedFileCount = skippedCount
	summary.FailedFileCount += failedCount

	for _, filePath := range resolvedFiles {
		if ctx != nil {
			select {
			case <-ctx.Done():
				log.Printf("%s-%s-CONTEXTDONE preconfiguration interrupted: %v", preconfigurationComponent, preconfigurationOperation, ctx.Err())
				return summary
			default:
			}
		}

		if err := importPreconfigurationFile(ctx, uploadService, filePath, &summary); err != nil {
			log.Printf("%s-%s-IMPORTFILE failed to import '%s': %v", preconfigurationComponent, preconfigurationOperation, sanitizeLogValue(filePath), err)
		}
	}

	log.Printf(
		"%s-%s-COMPLETED configured=%d resolved=%d imported=%d failed=%d skipped=%d",
		preconfigurationComponent,
		preconfigurationOperation,
		summary.ConfiguredSourceCount,
		summary.ResolvedFileCount,
		summary.ImportedFileCount,
		summary.FailedFileCount,
		summary.SkippedFileCount,
	)
	return summary
}

func importPreconfigurationFile(
	ctx context.Context,
	uploadService UploadService,
	filePath string,
	summary *PreconfigurationSummary,
) error {
	cleanPath := filepath.Clean(filePath)
	// #nosec G304 -- path is resolved from operator configuration, cleaned and extension-filtered.
	file, err := os.Open(cleanPath)
	if err != nil {
		summary.FailedFileCount++
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	response, importErr := uploadService.HandleUpload(ctx, filepath.Base(cleanPath), "", file)
	if importErr != nil {
		summary.FailedFileCount++
		return importErr
	}
	if response.Code >= http.StatusBadRequest {
		summary.FailedFileCount++
		return buildPreconfigurationUploadFailureError(response.Code, filepath.Base(cleanPath), response.Body)
	}

	summary.ImportedFileCount++
	return nil
}

func buildPreconfigurationUploadFailureError(status int, fileName string, responseBody any) error {
	sanitizedFileName := sanitizeLogValue(fileName)
	errorDetail := sanitizeLogValue(extractPreconfigurationUploadErrorDetail(responseBody))
	if errorDetail == "" {
		return fmt.Errorf(
			"AASENV-PRECONF-UPLOADFAILED upload handler returned status %d for file '%s'",
			status,
			sanitizedFileName,
		)
	}

	return fmt.Errorf(
		"AASENV-PRECONF-UPLOADFAILED upload handler returned status %d for file '%s': %s",
		status,
		sanitizedFileName,
		errorDetail,
	)
}

func extractPreconfigurationUploadErrorDetail(responseBody any) string {
	switch body := responseBody.(type) {
	case []common.ErrorHandler:
		if len(body) == 0 {
			return ""
		}
		return strings.TrimSpace(body[0].Text)
	case common.ErrorHandler:
		return strings.TrimSpace(body.Text)
	default:
		return ""
	}
}
func resolvePreconfigurationFiles(configuredSources []string) ([]string, int, int) {
	resolvedSet := make(map[string]struct{})
	skippedCount := 0
	failedCount := 0

	for _, rawSource := range configuredSources {
		sourcePath := normalizePreconfigurationSource(rawSource)
		if sourcePath == "" {
			skippedCount++
			continue
		}

		// #nosec G304 -- sourcePath is from operator configuration and only used for readonly file existence checks.
		sourceInfo, statErr := os.Stat(sourcePath)
		if statErr != nil {
			failedCount++
			log.Printf("%s-%s-INVALIDSOURCE source '%s' cannot be accessed: %v", preconfigurationComponent, preconfigurationOperation, sanitizeLogValue(sourcePath), statErr)
			continue
		}

		if sourceInfo.IsDir() {
			skippedInDir, failedInDir := collectPreconfigurationFilesFromDirectory(sourcePath, resolvedSet)
			skippedCount += skippedInDir
			failedCount += failedInDir
			continue
		}

		if !isSupportedAASPreconfigurationFile(sourcePath) {
			skippedCount++
			log.Printf("%s-%s-SKIPUNSUPPORTED skipped unsupported file '%s'", preconfigurationComponent, preconfigurationOperation, sanitizeLogValue(sourcePath))
			continue
		}

		resolvedSet[filepath.Clean(sourcePath)] = struct{}{}
	}

	resolvedFiles := make([]string, 0, len(resolvedSet))
	for filePath := range resolvedSet {
		resolvedFiles = append(resolvedFiles, filePath)
	}
	sort.Strings(resolvedFiles)

	return resolvedFiles, skippedCount, failedCount
}

func collectPreconfigurationFilesFromDirectory(dirPath string, resolvedSet map[string]struct{}) (int, int) {
	skippedCount := 0
	failedCount := 0

	walkErr := filepath.WalkDir(dirPath, func(path string, entry os.DirEntry, walkEntryErr error) error {
		if walkEntryErr != nil {
			failedCount++
			log.Printf("%s-%s-WALKDIR failed while reading '%s': %v", preconfigurationComponent, preconfigurationOperation, sanitizeLogValue(path), walkEntryErr)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !isSupportedAASPreconfigurationFile(path) {
			skippedCount++
			return nil
		}

		resolvedSet[filepath.Clean(path)] = struct{}{}
		return nil
	})
	if walkErr != nil {
		failedCount++
		log.Printf("%s-%s-WALKDIRFAILED unable to traverse '%s': %v", preconfigurationComponent, preconfigurationOperation, sanitizeLogValue(dirPath), walkErr)
	}

	return skippedCount, failedCount
}

func isSupportedAASPreconfigurationFile(filePath string) bool {
	extension := strings.ToLower(strings.TrimSpace(filepath.Ext(filePath)))
	_, supported := supportedAASPreconfigurationExtensions[extension]
	return supported
}

func normalizePreconfigurationSource(rawSource string) string {
	source := strings.TrimSpace(rawSource)
	if source == "" {
		return ""
	}

	if strings.HasPrefix(strings.ToLower(source), "file:") {
		source = strings.TrimSpace(source[5:])
	}

	if source == "" {
		return ""
	}

	return filepath.Clean(source)
}
