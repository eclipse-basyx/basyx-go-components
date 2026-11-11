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
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aasjsonization "github.com/FriedJannik/aas-go-sdk/jsonization"
	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestUploadProcessingStatus_PropagatesWrappedCommonErrors(t *testing.T) {
	testCases := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "bad request",
			err:    fmt.Errorf("wrap: %w", common.NewErrBadRequest("x")),
			status: http.StatusBadRequest,
		},
		{
			name:   "method not allowed",
			err:    fmt.Errorf("wrap: %w", common.NewErrMethodNotAllowed("x")),
			status: http.StatusMethodNotAllowed,
		},
		{
			name:   "not found",
			err:    fmt.Errorf("wrap: %w", common.NewErrNotFound("x")),
			status: http.StatusNotFound,
		},
		{
			name:   "denied",
			err:    fmt.Errorf("wrap: %w", common.NewErrDenied("x")),
			status: http.StatusForbidden,
		},
		{
			name:   "conflict",
			err:    fmt.Errorf("wrap: %w", common.NewErrConflict("x")),
			status: http.StatusConflict,
		},
		{
			name:   "internal",
			err:    fmt.Errorf("wrap: %w", common.NewInternalServerError("x")),
			status: http.StatusInternalServerError,
		},
		{
			name:   "unknown defaults to internal",
			err:    fmt.Errorf("random"),
			status: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := uploadProcessingStatus(tc.err)
			if got != tc.status {
				t.Fatalf("expected status %d, got %d for %v", tc.status, got, tc.err)
			}
		})
	}
}

func TestReadEnvironmentFromAASXSpec_AdaptsLegacyNamespace(t *testing.T) {
	hartingPath := filepath.Join("integration_tests", "testdata", "HARTING_AAS_09140009950.aasx")
	if _, err := os.Stat(hartingPath); err != nil {
		t.Fatalf("failed to access HARTING fixture: %v", err)
	}

	hartingFile, err := os.Open(hartingPath) // #nosec G304 -- test fixture path is static and controlled by repository sources.
	if err != nil {
		t.Fatalf("failed to open HARTING fixture: %v", err)
	}
	defer func() {
		_ = hartingFile.Close()
	}()

	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(hartingFile)
	if err != nil {
		t.Fatalf("failed to open AASX package: %v", err)
	}
	defer func() {
		_ = packageReader.Close()
	}()

	_, environment, parseErr := ReadEnvironmentFromAASXSpec(packageReader, filepath.Base(hartingPath))
	if parseErr != nil {
		t.Fatalf("expected legacy namespace to be adapted successfully, got error: %v", parseErr)
	}
	if environment == nil {
		t.Fatal("expected parsed environment, got nil")
	}
	if len(environment.AssetAdministrationShells()) == 0 {
		t.Fatal("expected parsed environment to contain at least one AAS")
	}
}

func TestParseAASXMLInstance_AdaptsLegacyNamespace(t *testing.T) {
	xmlPath := filepath.Join("integration_tests", "testdata", "environment_minimal.xml")
	specContent, err := os.ReadFile(xmlPath) // #nosec G304 -- test fixture path is static and controlled by repository sources.
	if err != nil {
		t.Fatalf("failed to read XML fixture: %v", err)
	}

	instance, parseErr := parseAASXMLInstance(specContent, filepath.Base(xmlPath))
	if parseErr != nil {
		t.Fatalf("expected legacy XML namespace to be adapted successfully, got error: %v", parseErr)
	}
	if instance == nil {
		t.Fatal("expected parsed XML instance, got nil")
	}
}

func TestDigestAASXThumbnailsRejectsOversizedThumbnailBeforeImport(t *testing.T) {
	const thumbnailPath = "/aasx/files/thumbnail.png"
	assetInformation := aastypes.NewAssetInformation(aastypes.AssetKindInstance)
	thumbnail := aastypes.NewResource(thumbnailPath)
	contentType := "image/png"
	thumbnail.SetContentType(&contentType)
	assetInformation.SetDefaultThumbnail(thumbnail)
	aas := aastypes.NewAssetAdministrationShell("aas-id", assetInformation)
	environment := aastypes.NewEnvironment()
	environment.SetAssetAdministrationShells([]aastypes.IAssetAdministrationShell{aas})
	jsonable, err := aasjsonization.ToJsonable(environment)
	if err != nil {
		t.Fatalf("failed to convert environment to JSON: %v", err)
	}
	specification, err := json.Marshal(jsonable)
	if err != nil {
		t.Fatalf("failed to marshal environment: %v", err)
	}
	thumbnailPart, err := buildSerializationThumbnailPart("aas-id", "thumbnail.png", contentType, thumbnailPath, []byte("oversized"))
	if err != nil {
		t.Fatalf("failed to build thumbnail part: %v", err)
	}
	payload, err := serializeEnvironmentToAASXPackage(
		specification, "application/json", serializationAASXJSONSpecURI,
		[]serializationThumbnailPart{thumbnailPart}, nil,
	)
	if err != nil {
		t.Fatalf("failed to build AASX package: %v", err)
	}
	packageReader, err := aasx.NewPackaging().OpenReadFromStream(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to open AASX package: %v", err)
	}
	defer func() { _ = packageReader.Close() }()
	specPart, parsedEnvironment, err := ReadEnvironmentFromAASXSpec(packageReader, "environment.aasx")
	if err != nil {
		t.Fatalf("failed to parse AASX environment: %v", err)
	}
	ctx := common.ContextWithConfig(t.Context(), &common.Config{General: common.GeneralConfig{AASXMaxThumbnailSizeBytes: 4}})

	_, err = digestAASXThumbnails(ctx, packageReader, specPart, parsedEnvironment)

	if !common.IsErrPayloadTooLarge(err) {
		t.Fatalf("expected payload-too-large error, got %v", err)
	}
}

func TestWarnFileReferencesWithoutSupplementaryRelationshipReportsOnlyUnrelatedPackageParts(t *testing.T) {
	var destination bytes.Buffer
	writer, err := aasx.NewPackaging().CreateWriter(&destination)
	if err != nil {
		t.Fatalf("failed to create AASX writer: %v", err)
	}
	specURI := mustParseTestURL(t, "/aasx/environment.json")
	specPart, err := writer.PutPartFromStream(specURI, "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("failed to put spec part: %v", err)
	}
	if err = writer.MakeSpec(specPart); err != nil {
		t.Fatalf("failed to mark spec part: %v", err)
	}
	relatedPart, err := writer.PutPartFromStream(mustParseTestURL(t, "/aasx/files/related.png"), "image/png", bytes.NewReader([]byte("related")))
	if err != nil {
		t.Fatalf("failed to put related part: %v", err)
	}
	if err = writer.RelateSupplementaryToSpec(relatedPart, specPart); err != nil {
		t.Fatalf("failed to relate supplementary part: %v", err)
	}
	if _, err = writer.PutPartFromStream(mustParseTestURL(t, "/aasx/files/unrelated.jpg"), "image/jpeg", bytes.NewReader([]byte("unrelated"))); err != nil {
		t.Fatalf("failed to put unrelated part: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("failed to close AASX writer: %v", err)
	}
	packageReader, err := aasx.NewPackaging().OpenReadFromStream(bytes.NewReader(destination.Bytes()))
	if err != nil {
		t.Fatalf("failed to open AASX package: %v", err)
	}
	defer func() { _ = packageReader.Close() }()

	locations := []AASXFileElementLocation{
		{SubmodelID: "sm", IDShortPath: "Images[0]", FileValue: "/aasx/files/unrelated.jpg"},
		{SubmodelID: "sm", IDShortPath: "Logo", FileValue: "/aasx/files/related.png"},
		{SubmodelID: "sm", IDShortPath: "Missing", FileValue: "/aasx/files/missing.png"},
		{SubmodelID: "sm", IDShortPath: "External", FileValue: "https://example.com/image.png"},
	}
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previousLogger)

	warnFileReferencesWithoutSupplementaryRelationship(t.Context(), packageReader, specURI, locations, map[int]struct{}{1: {}})

	logLines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(logLines) != 1 {
		t.Fatalf("expected exactly one warning, got %d: %s", len(logLines), logs.String())
	}
	for _, expected := range []string{`"error.code":"AASENV-UPLDSUPPL-NOSUPPLREL"`, `"id_short_path":"Images[0]"`, `"part_uri":"/aasx/files/unrelated.jpg"`} {
		if !strings.Contains(logLines[0], expected) {
			t.Fatalf("expected warning to contain %s, got %s", expected, logLines[0])
		}
	}
}

func mustParseTestURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("failed to parse URL %q: %v", rawURL, err)
	}
	return parsed
}
