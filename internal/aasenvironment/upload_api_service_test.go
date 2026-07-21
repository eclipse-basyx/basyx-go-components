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
	"net/http"
	"os"
	"path/filepath"
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

	_, environment, parseErr := readEnvironmentFromAASXSpec(packageReader, filepath.Base(hartingPath))
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

func TestValidateAASXThumbnailLimitsRejectsOversizedThumbnailBeforeImport(t *testing.T) {
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
	specPart, parsedEnvironment, err := readEnvironmentFromAASXSpec(packageReader, "environment.aasx")
	if err != nil {
		t.Fatalf("failed to parse AASX environment: %v", err)
	}
	ctx := common.ContextWithConfig(t.Context(), &common.Config{General: common.GeneralConfig{AASXMaxThumbnailSizeBytes: 4}})

	err = validateAASXThumbnailLimits(ctx, packageReader, specPart, parsedEnvironment)

	if !common.IsErrPayloadTooLarge(err) {
		t.Fatalf("expected payload-too-large error, got %v", err)
	}
}
