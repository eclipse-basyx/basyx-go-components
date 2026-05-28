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

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	aasjsonization "github.com/FriedJannik/aas-go-sdk/jsonization"
	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	aasxmlization "github.com/FriedJannik/aas-go-sdk/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/stretchr/testify/require"
)

const (
	fixtureAasxFilePathThreeAasXml                     = "testdata/threeAasDuplicateFilesSerializationTestXml.aasx"
	serializationThreeAasxXmlDownloadPath              = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_from_xml_aasx_upload.aasx"
	serializationThreeAasxXmlAltDownloadPath           = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_alt_from_xml_aasx_upload.aasx"
	serializationThreeAasxJsonDownloadPath             = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_from_xml_aasx_upload.aasx"
	serializationThreeAasxJsonAltDownloadPath          = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_alt_from_xml_aasx_upload.aasx"
	serializationThreeAasXmlDownloadFromXmlUploadPath  = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_from_xml_aasx_upload.xml"
	serializationThreeAasJsonDownloadFromXmlUploadPath = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_from_xml_aasx_upload.json"
	serializationBaseURL                               = "http://127.0.0.1:6004"

	serializationIntegrationDSN = "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable"
)

func TestSerializationDownloadAasXmlAfterThreeAasUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		accept     string
		outputPath string
	}{
		{
			name:       "AASXXMLFromAASXXMLUpload",
			accept:     "application/aasx+xml",
			outputPath: serializationThreeAasxXmlDownloadPath,
		},
		{
			name:       "AssetAdministrationShellXMLFromAASXXMLUpload",
			accept:     "application/asset-administration-shell+xml",
			outputPath: serializationThreeAasxXmlAltDownloadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, fixtureAasxFilePathThreeAasXml, "application/aasx+xml")

			payload := downloadAASXSerializationFullEnvironment(t, testCase.accept, true)
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded AASX XML serialization for Accept %q to %s", testCase.accept, testCase.outputPath)
		})
	}
}

func TestSerializationDownloadAasxJsonAfterThreeAASUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		accept     string
		outputPath string
	}{
		{
			name:       "AASXJSONFromAASXXMLUpload",
			accept:     "application/aasx+json",
			outputPath: serializationThreeAasxJsonDownloadPath,
		},
		{
			name:       "AssetAdministrationShellJSONFromAASXXMLUpload",
			accept:     "application/asset-administration-shell+json",
			outputPath: serializationThreeAasxJsonAltDownloadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, fixtureAasxFilePathThreeAasXml, "application/aasx+xml")

			payload := downloadAASXSerializationFullEnvironment(t, testCase.accept, true)
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded AASX JSON serialization for Accept %q to %s", testCase.accept, testCase.outputPath)
		})
	}
}

func TestSerializationDownloadXmlAfterThreeAasUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		outputPath string
	}{
		{
			name:       "XMLFromAASXXMLUpload",
			outputPath: serializationThreeAasXmlDownloadFromXmlUploadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, fixtureAasxFilePathThreeAasXml, "application/aasx+xml")

			payload := downloadAASXSerializationFullEnvironment(t, "application/xml", true)
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded XML serialization to %s", testCase.outputPath)
		})
	}
}

func TestSerializationDownloadJsonAfterThreeAasUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		outputPath string
	}{
		{
			name:       "JSONFromAASXXMLUpload",
			outputPath: serializationThreeAasJsonDownloadFromXmlUploadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, fixtureAasxFilePathThreeAasXml, "application/aasx+xml")

			payload := downloadAASXSerializationFullEnvironment(t, "application/json", true)
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded JSON serialization to %s", testCase.outputPath)
		})
	}
}

func TestSerializationDownloadAASXXMLAfterThreeAASUploadRequestedAcceptFormat(t *testing.T) {
	testCases := []struct {
		name           string
		accept         string
		expectedFormat string
	}{
		{
			name:           "AASXXML",
			accept:         "application/aasx+xml",
			expectedFormat: "aasx-xml",
		},
		{
			name:           "AASXJSON",
			accept:         "application/aasx+json",
			expectedFormat: "aasx-json",
		},
		{
			name:           "AASXXMLAlt",
			accept:         "application/asset-administration-shell+xml",
			expectedFormat: "aasx-xml",
		},
		{
			name:           "AASXJSONAlt",
			accept:         "application/asset-administration-shell+json",
			expectedFormat: "aasx-json",
		},
		{
			name:           "PlainXML",
			accept:         "application/xml",
			expectedFormat: "plain-xml",
		},
		{
			name:           "PlainJSON",
			accept:         "application/json",
			expectedFormat: "plain-json",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, fixtureAasxFilePathThreeAasXml, "application/aasx+xml")

			downloadPayload := downloadAASXSerializationFullEnvironment(t, testCase.accept, true)
			require.NotEmpty(t, downloadPayload)

			switch testCase.expectedFormat {
			case "aasx-xml":
				downloadedSpecParts := readAASXSpecPartsFromPayload(t, downloadPayload)
				require.NotEmptyf(t, downloadedSpecParts, "AASX payload for Accept %q has no spec parts", testCase.accept)
				for _, specPart := range downloadedSpecParts {
					require.Truef(t, isXMLSpecPart(specPart), "AASX spec part for Accept %q must be XML", testCase.accept)
				}

			case "aasx-json":
				downloadedSpecParts := readAASXSpecPartsFromPayload(t, downloadPayload)
				require.NotEmptyf(t, downloadedSpecParts, "AASX payload for Accept %q has no spec parts", testCase.accept)
				for _, specPart := range downloadedSpecParts {
					require.Truef(t, isJSONSpecPart(specPart), "AASX spec part for Accept %q must be JSON", testCase.accept)
				}

			case "plain-xml":
				environment, transformToEnvErr := environmentFromPlainSerializationPayload(downloadPayload, "application/xml")
				require.NoErrorf(t, transformToEnvErr, "plain XML payload for Accept %q could not be parsed", testCase.accept)
				require.NotNil(t, environment)

			case "plain-json":
				environment, transformToEnvErr := environmentFromPlainSerializationPayload(downloadPayload, "application/json")
				require.NoErrorf(t, transformToEnvErr, "plain JSON payload for Accept %q could not be parsed", testCase.accept)
				require.NotNil(t, environment)

			default:
				t.Fatalf("unsupported expected format %q", testCase.expectedFormat)
			}
		})
	}
}

func TestSerializationDownloadAASXWithoutUploadContainsSameIds(t *testing.T) {
	testCases := []struct {
		name   string
		accept string
	}{
		{name: "AASXXML", accept: "application/aasx+xml"},
		{name: "AASXJSON", accept: "application/aasx+json"},
		{name: "AASXXMLAlt", accept: "application/asset-administration-shell+xml"},
		{name: "AASXJSONAlt", accept: "application/asset-administration-shell+json"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)

			postJSONFixture(t, serializationBaseURL+"/shells", "testdata/serialization_asset_administration_shell.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/submodels", "testdata/serialization_submodel.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/concept-descriptions", "testdata/serialization_concept_description.json", http.StatusCreated)

			payload := downloadAASXSerializationFullEnvironment(t, testCase.accept, true)
			require.NotEmpty(t, payload)

			specParts := readAASXSpecPartsFromPayload(t, payload)
			require.NotEmptyf(t, specParts, "serialization payload without upload must contain at least one AASX spec part for Accept %q", testCase.accept)

			environment, transformToEnvErr := environmentFromAASXSpecParts(specParts)
			require.NoError(t, transformToEnvErr)

			require.NoError(t, requireEnvironmentContainsAllIDs(
				environment,
				[]string{"urn:fraunhofer:iese:dte:aas:drivemotor-dm3000:001"},
				[]string{"urn:fraunhofer:iese:dte:sm:carbonfootprint:drivemotor-dm3000:001"},
				[]string{"0173-1#02-ABG776#003"},
			))
		})
	}
}

func TestSerializationDownloadAASXWithoutUploadContainsSubsetIds(t *testing.T) {
	testCases := []struct {
		name   string
		accept string
	}{
		{name: "AASXXML", accept: "application/aasx+xml"},
		{name: "AASXJSON", accept: "application/aasx+json"},
		{name: "AASXXMLAlt", accept: "application/asset-administration-shell+xml"},
		{name: "AASXJSONAlt", accept: "application/asset-administration-shell+json"},
	}

	expectedAASID := "urn:example:aas:registry-sync-post"
	expectedSubmodelID := "urn:example:sm:registry-sync-post"

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)

			postJSONFixture(t, serializationBaseURL+"/shells", "testdata/serialization_asset_administration_shell.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/shells", "testdata/registry_sync_post_shell.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/submodels", "testdata/serialization_submodel.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/submodels", "testdata/registry_sync_post_submodel.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/concept-descriptions", "testdata/serialization_concept_description.json", http.StatusCreated)

			payload := downloadAASXSerializationFilteredEnvironment(
				t,
				testCase.accept,
				[]string{expectedAASID},
				[]string{expectedSubmodelID},
				false,
			)
			require.NotEmpty(t, payload)

			specParts := readAASXSpecPartsFromPayload(t, payload)
			require.NotEmptyf(t, specParts, "filtered serialization payload must contain at least one AASX spec part for Accept %q", testCase.accept)

			environment, transformToEnvErr := environmentFromAASXSpecParts(specParts)
			require.NoError(t, transformToEnvErr)

			require.NoError(t, requireEnvironmentContainsAllIDs(
				environment,
				[]string{expectedAASID},
				[]string{expectedSubmodelID},
				nil,
			))

			require.Equal(t, []string{expectedAASID}, environmentAASIDs(environment))
			require.Equal(t, []string{expectedSubmodelID}, environmentSubmodelIDs(environment))
			require.Empty(t, environmentConceptDescriptionIDs(environment))
		})
	}
}

func TestSerializationDownloadPlainWithoutUploadContainsSameIds(t *testing.T) {
	testCases := []struct {
		name   string
		accept string
	}{
		{name: "PlainXML", accept: "application/xml"},
		{name: "PlainJSON", accept: "application/json"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)

			postJSONFixture(t, serializationBaseURL+"/shells", "testdata/serialization_asset_administration_shell.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/submodels", "testdata/serialization_submodel.json", http.StatusCreated)
			postJSONFixture(t, serializationBaseURL+"/concept-descriptions", "testdata/serialization_concept_description.json", http.StatusCreated)

			payload := downloadAASXSerializationFullEnvironment(t, testCase.accept, true)
			require.NotEmpty(t, payload)

			environment, transformToEnvErr := environmentFromPlainSerializationPayload(payload, testCase.accept)
			require.NoError(t, transformToEnvErr)

			require.NoError(t, requireEnvironmentContainsAllIDs(
				environment,
				[]string{"urn:fraunhofer:iese:dte:aas:drivemotor-dm3000:001"},
				[]string{"urn:fraunhofer:iese:dte:sm:carbonfootprint:drivemotor-dm3000:001"},
				[]string{"0173-1#02-ABG776#003"},
			))
		})
	}
}

func TestSerializationDownloadAASXAfterUploadContainsSameFiles(t *testing.T) {
	resetDatabaseForUploadIT(t, serializationIntegrationDSN)

	uploadPath := "testdata/IESEDriveMotorDM3000.aasx"
	uploadFixture(t, uploadPath, "application/aasx+xml")

	downloadPayload := downloadAASXSerializationFullEnvironment(t, "application/aasx+xml", true)
	require.NotEmpty(t, downloadPayload)

	uploadedFingerprints, uploadedErr := aasxSupplementaryFingerprintsFromFile(uploadPath)
	require.NoError(t, uploadedErr)
	require.NotEmpty(t, uploadedFingerprints)

	downloadedFingerprints, downloadedErr := aasxSupplementaryFingerprintsFromBytes(downloadPayload)
	require.NoError(t, downloadedErr)
	require.NotEmpty(t, downloadedFingerprints)

	uploadedFileHashes := hashesFromSupplementaryFingerprints(uploadedFingerprints)
	downloadedFileHashes := hashesFromSupplementaryFingerprints(downloadedFingerprints)

	uploadedFingerprintDetails := formatSupplementaryFingerprintDetails(uploadedFingerprints)
	downloadedFingerprintDetails := formatSupplementaryFingerprintDetails(downloadedFingerprints)
	t.Logf("uploaded AASX supplementary files (name | hash | uri):\n%s", uploadedFingerprintDetails)
	t.Logf("downloaded AASX supplementary files (name | hash | uri):\n%s", downloadedFingerprintDetails)

	require.Equalf(
		t,
		uploadedFileHashes,
		downloadedFileHashes,
		"downloaded AASX supplementary contents differ from uploaded AASX\nuploaded files:\n%s\ndownloaded files:\n%s",
		uploadedFingerprintDetails,
		downloadedFingerprintDetails,
	)

	uploadedThumbnail, uploadedThumbnailErr := aasxThumbnailFingerprintFromFile(uploadPath)
	require.NoError(t, uploadedThumbnailErr)

	downloadedThumbnail, downloadedThumbnailErr := aasxThumbnailFingerprintFromBytes(downloadPayload)
	require.NoError(t, downloadedThumbnailErr)

	require.Equal(t, uploadedThumbnail != nil, downloadedThumbnail != nil, "thumbnail presence differs between uploaded and downloaded AASX")
	if uploadedThumbnail != nil && downloadedThumbnail != nil {
		uploadedThumbnailDetails := formatSupplementaryFingerprintDetails([]aasxSupplementaryFingerprint{*uploadedThumbnail})
		downloadedThumbnailDetails := formatSupplementaryFingerprintDetails([]aasxSupplementaryFingerprint{*downloadedThumbnail})
		t.Logf("uploaded AASX thumbnail (name | hash | uri):\n%s", uploadedThumbnailDetails)
		t.Logf("downloaded AASX thumbnail (name | hash | uri):\n%s", downloadedThumbnailDetails)

		require.Equalf(
			t,
			uploadedThumbnail.Hash,
			downloadedThumbnail.Hash,
			"downloaded AASX thumbnail differs from uploaded AASX thumbnail\nuploaded thumbnail:\n%s\ndownloaded thumbnail:\n%s",
			uploadedThumbnailDetails,
			downloadedThumbnailDetails,
		)
	}
}

func TestSerializationDownloadReturnsStatusOK(t *testing.T) {
	resetDatabaseForUploadIT(t, serializationIntegrationDSN)

	postJSONFixture(t, serializationBaseURL+"/shells", "testdata/serialization_asset_administration_shell.json", http.StatusCreated)
	postJSONFixture(t, serializationBaseURL+"/submodels", "testdata/serialization_submodel.json", http.StatusCreated)

	query := url.Values{}
	query.Set("includeConceptDescriptions", strconv.FormatBool(false))

	req, err := http.NewRequest(http.MethodGet, serializationBaseURL+"/serialization?"+query.Encode(), nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	require.NotEmpty(t, body, "successful serialization should return a non-empty payload")

	require.Equal(t, http.StatusOK, resp.StatusCode, "successful serialization should return HTTP 200")
}

func requireEnvironmentContainsAllIDs(environment aastypes.IEnvironment, aasIDs []string, submodelIDs []string, conceptDescriptionIDs []string) error {
	if environment == nil {
		return fmt.Errorf("environment is nil")
	}

	availableAASIDs := make(map[string]struct{}, len(environment.AssetAdministrationShells()))
	for _, aas := range environment.AssetAdministrationShells() {
		if aas == nil {
			continue
		}
		id := strings.TrimSpace(aas.ID())
		if id != "" {
			availableAASIDs[id] = struct{}{}
		}
	}

	availableSubmodelIDs := make(map[string]struct{}, len(environment.Submodels()))
	for _, submodel := range environment.Submodels() {
		if submodel == nil {
			continue
		}
		id := strings.TrimSpace(submodel.ID())
		if id != "" {
			availableSubmodelIDs[id] = struct{}{}
		}
	}

	availableConceptDescriptionIDs := make(map[string]struct{}, len(environment.ConceptDescriptions()))
	for _, conceptDescription := range environment.ConceptDescriptions() {
		if conceptDescription == nil {
			continue
		}
		id := strings.TrimSpace(conceptDescription.ID())
		if id != "" {
			availableConceptDescriptionIDs[id] = struct{}{}
		}
	}

	missingAASIDs := missingIDs(aasIDs, availableAASIDs)
	missingSubmodelIDs := missingIDs(submodelIDs, availableSubmodelIDs)
	missingConceptDescriptionIDs := missingIDs(conceptDescriptionIDs, availableConceptDescriptionIDs)

	missingGroups := make([]string, 0, 3)
	if len(missingAASIDs) > 0 {
		missingGroups = append(missingGroups, fmt.Sprintf("aas=%v", missingAASIDs))
	}
	if len(missingSubmodelIDs) > 0 {
		missingGroups = append(missingGroups, fmt.Sprintf("submodels=%v", missingSubmodelIDs))
	}
	if len(missingConceptDescriptionIDs) > 0 {
		missingGroups = append(missingGroups, fmt.Sprintf("conceptDescriptions=%v", missingConceptDescriptionIDs))
	}

	if len(missingGroups) > 0 {
		return fmt.Errorf("missing ids in environment: %s", strings.Join(missingGroups, ", "))
	}

	return nil
}

func missingIDs(expectedIDs []string, availableIDs map[string]struct{}) []string {
	missing := make([]string, 0)
	seen := make(map[string]struct{}, len(expectedIDs))
	for _, expectedID := range expectedIDs {
		trimmedID := strings.TrimSpace(expectedID)
		if trimmedID == "" {
			continue
		}
		if _, alreadySeen := seen[trimmedID]; alreadySeen {
			continue
		}
		seen[trimmedID] = struct{}{}
		if _, found := availableIDs[trimmedID]; !found {
			missing = append(missing, trimmedID)
		}
	}

	sort.Strings(missing)
	return missing
}

func environmentAASIDs(environment aastypes.IEnvironment) []string {
	if environment == nil {
		return nil
	}

	ids := make([]string, 0, len(environment.AssetAdministrationShells()))
	for _, aas := range environment.AssetAdministrationShells() {
		if aas == nil {
			continue
		}
		ids = append(ids, aas.ID())
	}

	return sortedUniqueIDs(ids)
}

func environmentSubmodelIDs(environment aastypes.IEnvironment) []string {
	if environment == nil {
		return nil
	}

	ids := make([]string, 0, len(environment.Submodels()))
	for _, submodel := range environment.Submodels() {
		if submodel == nil {
			continue
		}
		ids = append(ids, submodel.ID())
	}

	return sortedUniqueIDs(ids)
}

func environmentConceptDescriptionIDs(environment aastypes.IEnvironment) []string {
	if environment == nil {
		return nil
	}

	ids := make([]string, 0, len(environment.ConceptDescriptions()))
	for _, conceptDescription := range environment.ConceptDescriptions() {
		if conceptDescription == nil {
			continue
		}
		ids = append(ids, conceptDescription.ID())
	}

	return sortedUniqueIDs(ids)
}

func sortedUniqueIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		trimmedID := strings.TrimSpace(id)
		if trimmedID == "" {
			continue
		}
		if _, exists := seen[trimmedID]; exists {
			continue
		}
		seen[trimmedID] = struct{}{}
		result = append(result, trimmedID)
	}

	sort.Strings(result)
	return result
}

type aasxSupplementaryFingerprint struct {
	Name string
	URI  string
	Hash string
}

func aasxSupplementaryFingerprintsFromFile(filePath string) ([]aasxSupplementaryFingerprint, error) {
	payload, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}

	return aasxSupplementaryFingerprintsFromBytes(payload)
}

func aasxSupplementaryFingerprintsFromBytes(payload []byte) ([]aasxSupplementaryFingerprint, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("payload is empty")
	}

	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = packageReader.Close() }()

	relationships, err := packageReader.SupplementaryRelationships()
	if err != nil {
		return nil, err
	}
	thumbnailPart, err := packageReader.Thumbnail()
	if err != nil {
		return nil, err
	}
	normalizedThumbnailURI := normalizedAASXPartURI(thumbnailPart)

	fingerprints := make([]aasxSupplementaryFingerprint, 0, len(relationships))
	seenSupplementaryURIs := make(map[string]struct{}, len(relationships))
	for _, relationship := range relationships {
		if relationship == nil || relationship.Supplementary == nil {
			continue
		}
		if normalizedThumbnailURI != "" && normalizedAASXPartURI(relationship.Supplementary) == normalizedThumbnailURI {
			continue
		}

		fingerprints, err = appendAASXPartFingerprint(fingerprints, seenSupplementaryURIs, relationship.Supplementary)
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(fingerprints, func(i, j int) bool {
		if fingerprints[i].Name != fingerprints[j].Name {
			return fingerprints[i].Name < fingerprints[j].Name
		}
		if fingerprints[i].Hash != fingerprints[j].Hash {
			return fingerprints[i].Hash < fingerprints[j].Hash
		}
		return fingerprints[i].URI < fingerprints[j].URI
	})

	return fingerprints, nil
}

func aasxThumbnailFingerprintFromFile(filePath string) (*aasxSupplementaryFingerprint, error) {
	payload, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}

	return aasxThumbnailFingerprintFromBytes(payload)
}

func aasxThumbnailFingerprintFromBytes(payload []byte) (*aasxSupplementaryFingerprint, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("payload is empty")
	}

	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = packageReader.Close() }()

	thumbnailPart, err := packageReader.Thumbnail()
	if err != nil {
		return nil, err
	}

	return aasxPartFingerprint(thumbnailPart)
}

func appendAASXPartFingerprint(
	fingerprints []aasxSupplementaryFingerprint,
	seenSupplementaryURIs map[string]struct{},
	part *aasx.Part,
) ([]aasxSupplementaryFingerprint, error) {
	if part == nil {
		return fingerprints, nil
	}

	normalizedPartURI := normalizedAASXPartURI(part)
	if normalizedPartURI != "" {
		if _, alreadySeen := seenSupplementaryURIs[normalizedPartURI]; alreadySeen {
			return fingerprints, nil
		}
		seenSupplementaryURIs[normalizedPartURI] = struct{}{}
	}

	partFingerprint, err := aasxPartFingerprint(part)
	if err != nil {
		return nil, err
	}
	if partFingerprint == nil {
		return fingerprints, nil
	}

	fingerprints = append(fingerprints, *partFingerprint)

	return fingerprints, nil
}

func aasxPartFingerprint(part *aasx.Part) (*aasxSupplementaryFingerprint, error) {
	if part == nil {
		return nil, nil
	}

	partURI := ""
	if part.URI != nil {
		partURI = strings.TrimSpace(part.URI.String())
	}

	partContent, err := part.ReadAllBytes()
	if err != nil {
		return nil, err
	}
	if len(partContent) == 0 {
		return nil, nil
	}

	hash := sha256.Sum256(partContent)
	return &aasxSupplementaryFingerprint{
		Name: humanReadableSupplementaryName(partURI),
		URI:  partURI,
		Hash: fmt.Sprintf("%x", hash),
	}, nil
}

func normalizedAASXPartURI(part *aasx.Part) string {
	if part == nil || part.URI == nil {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(part.URI.String()))
}

func hashesFromSupplementaryFingerprints(fingerprints []aasxSupplementaryFingerprint) []string {
	hashes := make([]string, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		if strings.TrimSpace(fingerprint.Hash) == "" {
			continue
		}
		hashes = append(hashes, fingerprint.Hash)
	}
	sort.Strings(hashes)

	return hashes
}

func formatSupplementaryFingerprintDetails(fingerprints []aasxSupplementaryFingerprint) string {
	if len(fingerprints) == 0 {
		return "<none>"
	}

	lines := make([]string, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		name := strings.TrimSpace(fingerprint.Name)
		if name == "" {
			name = "<unknown-name>"
		}

		uri := strings.TrimSpace(fingerprint.URI)
		if uri == "" {
			uri = "<unknown-uri>"
		}

		lines = append(lines, fmt.Sprintf("%s | %s | %s", name, fingerprint.Hash, uri))
	}

	return strings.Join(lines, "\n")
}

func humanReadableSupplementaryName(uri string) string {
	normalizedURI := strings.TrimSpace(strings.ReplaceAll(uri, "\\", "/"))
	normalizedURI = strings.TrimPrefix(normalizedURI, "/")
	if normalizedURI == "" {
		return "<unknown-name>"
	}

	if decodedURI, err := url.PathUnescape(normalizedURI); err == nil && strings.TrimSpace(decodedURI) != "" {
		normalizedURI = decodedURI
	}

	baseName := strings.TrimSpace(path.Base(normalizedURI))
	if baseName == "" || baseName == "." || baseName == "/" {
		return normalizedURI
	}

	return baseName
}

func readAASXSpecPartsFromPayload(t *testing.T, payload []byte) []*aasx.Part {
	t.Helper()

	tempPath := filepath.Join(t.TempDir(), "serialization_result.aasx")
	require.NoError(t, os.WriteFile(tempPath, payload, 0o600))

	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenRead(tempPath)
	require.NoErrorf(t, err, "failed to open serialization payload as AASX package from %q", tempPath)
	defer func() { _ = packageReader.Close() }()

	specParts, err := packageReader.Specs()
	require.NoError(t, err)

	return specParts
}

// environmentFromAASXSpecParts converts AASX XML/JSON spec parts into an AAS
// environment object.
func environmentFromAASXSpecParts(specParts []*aasx.Part) (aastypes.IEnvironment, error) {
	if len(specParts) == 0 {
		return nil, fmt.Errorf("no spec parts provided")
	}

	for _, specPart := range specParts {
		if specPart == nil {
			continue
		}

		specContent, readErr := specPart.ReadAllBytes()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read AASX spec content: %w", readErr)
		}

		if isJSONSpecPart(specPart) {
			jsonable := any(nil)
			if unmarshalErr := json.Unmarshal(specContent, &jsonable); unmarshalErr != nil {
				return nil, fmt.Errorf("failed to unmarshal JSON spec: %w", unmarshalErr)
			}

			environment, parseErr := aasjsonization.EnvironmentFromJsonable(jsonable)
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse environment from JSON spec: %w", parseErr)
			}
			return environment, nil
		}

		if isXMLSpecPart(specPart) {
			instance, parseErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(specContent)))
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse XML spec: %w", parseErr)
			}

			environment, ok := instance.(aastypes.IEnvironment)
			if !ok {
				return nil, fmt.Errorf("xml spec root is %T, expected AAS environment", instance)
			}

			return environment, nil
		}
	}

	return nil, fmt.Errorf("no supported AASX XML/JSON spec part found")
}

func environmentFromPlainSerializationPayload(payload []byte, acceptHeader string) (aastypes.IEnvironment, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("payload is empty")
	}

	switch strings.ToLower(strings.TrimSpace(acceptHeader)) {
	case "application/json":
		jsonable := any(nil)
		if unmarshalErr := json.Unmarshal(payload, &jsonable); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON payload: %w", unmarshalErr)
		}

		environment, parseErr := aasjsonization.EnvironmentFromJsonable(jsonable)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse environment from JSON payload: %w", parseErr)
		}
		return environment, nil

	case "application/xml":
		instance, parseErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(payload)))
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse XML payload: %w", parseErr)
		}

		environment, ok := instance.(aastypes.IEnvironment)
		if !ok {
			return nil, fmt.Errorf("xml payload root is %T, expected AAS environment", instance)
		}

		return environment, nil
	default:
		return nil, fmt.Errorf("unsupported plain serialization accept header %q", acceptHeader)
	}
}

func isJSONSpecPart(specPart *aasx.Part) bool {
	if specPart == nil {
		return false
	}

	uri := ""
	if specPart.URI != nil {
		uri = strings.ToLower(strings.TrimSpace(specPart.URI.String()))
	}
	contentType := strings.ToLower(strings.TrimSpace(specPart.ContentType))

	return strings.HasSuffix(uri, ".json") || strings.Contains(contentType, "json")
}

func isXMLSpecPart(specPart *aasx.Part) bool {
	if specPart == nil {
		return false
	}

	uri := ""
	if specPart.URI != nil {
		uri = strings.ToLower(strings.TrimSpace(specPart.URI.String()))
	}
	contentType := strings.ToLower(strings.TrimSpace(specPart.ContentType))

	return strings.HasSuffix(uri, ".xml") || strings.Contains(contentType, "xml")
}

func postJSONFixture(t *testing.T, endpoint string, fixturePath string, expectedStatus int) {
	t.Helper()

	body, err := os.ReadFile(filepath.Clean(fixturePath))
	require.NoErrorf(t, err, "failed to read fixture %q", fixturePath)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, expectedStatus, resp.StatusCode, "request to %q with fixture %q failed: %s", endpoint, fixturePath, string(respBody))
}

func writeSerializationOutput(t *testing.T, outputPath string, payload []byte) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(outputPath), 0o750))
	require.NoError(t, os.WriteFile(outputPath, payload, 0o600))
}

func skipSerializationTestsInCI(t *testing.T) {
	t.Helper()

	if isTruthyEnv("CI") || isTruthyEnv("GITHUB_ACTIONS") {
		t.Skip("serialization download integration tests are local-only and skipped in CI")
	}
}

func isTruthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func uploadFixture(t *testing.T, fixturePath string, partContentType string) {
	t.Helper()

	file, err := os.Open(filepath.Clean(fixturePath))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q; filename=%q", "file", filepath.Base(fixturePath)))
	partHeader.Set("Content-Type", partContentType)

	part, err := writer.CreatePart(partHeader)
	require.NoError(t, err)

	_, err = io.Copy(part, file)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, serializationBaseURL+"/upload", payload)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "upload failed: %s", string(body))
}

func downloadAASXSerializationFullEnvironment(t *testing.T, acceptHeader string, includeConceptDescriptions bool) []byte {
	t.Helper()

	query := url.Values{}
	query.Set("includeConceptDescriptions", strconv.FormatBool(includeConceptDescriptions))

	req, err := http.NewRequest(http.MethodGet, serializationBaseURL+"/serialization?"+query.Encode(), nil)
	require.NoError(t, err)
	req.Header.Set("Accept", acceptHeader)

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "serialization full environment request failed for Accept %q: %s", acceptHeader, string(body))

	return body
}

func downloadAASXSerializationFilteredEnvironment(t *testing.T, acceptHeader string, aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) []byte {
	t.Helper()

	query := url.Values{}
	query.Set("includeConceptDescriptions", strconv.FormatBool(includeConceptDescriptions))

	for _, aasID := range aasIDs {
		trimmedID := strings.TrimSpace(aasID)
		if trimmedID == "" {
			continue
		}
		query.Add("aasIds", base64.RawURLEncoding.EncodeToString([]byte(trimmedID)))
	}

	for _, submodelID := range submodelIDs {
		trimmedID := strings.TrimSpace(submodelID)
		if trimmedID == "" {
			continue
		}
		query.Add("submodelIds", base64.RawURLEncoding.EncodeToString([]byte(trimmedID)))
	}

	req, err := http.NewRequest(http.MethodGet, serializationBaseURL+"/serialization?"+query.Encode(), nil)
	require.NoError(t, err)
	req.Header.Set("Accept", acceptHeader)

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "filtered serialization request failed for Accept %q: %s", acceptHeader, string(body))

	return body
}
