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
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

const (
	atomicUploadConceptDescriptionID = "urn:basyx:integration:atomic-upload:concept-description"
	atomicUploadSubmodelID           = "urn:basyx:integration:atomic-upload:submodel"
	atomicUploadShellID              = "urn:basyx:integration:atomic-upload:shell"
	atomicUploadSecondShellID        = "urn:basyx:integration:atomic-upload:second-shell"
	atomicUploadRejectedAttachment   = "rejected%25name.txt"
)

var atomicUploadThumbnail = []byte("\x89PNG\r\n\x1a\natomic upload thumbnail")

type atomicUploadPersistedState struct {
	LargeObjects        int64
	BinaryContents      int64
	FileReferences      int64
	ThumbnailReferences int64
	SubmodelDescriptors int64
	ShellDescriptors    int64
	ConceptDescriptions int64
	Submodels           int64
	Shells              int64
}

func TestUploadAASXStoresCompleteEnvironment(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)

	status, body := uploadAASXPayload(t, buildAtomicUploadPackage(t, "Shell", false), "atomic-upload.aasx")
	require.Equal(t, http.StatusOK, status, string(body))

	requireAtomicUploadResourceStatus(t, http.StatusOK)
	requireUploadStatus(t, "/submodels/"+encodeAtomicUploadID(atomicUploadSubmodelID)+"/submodel-elements/Manual/attachment", http.StatusOK)
	requireUploadStatus(t, "/shells/"+encodeAtomicUploadID(atomicUploadShellID)+"/asset-information/thumbnail", http.StatusOK)
	state := readAtomicUploadPersistedState(t)
	require.Equal(t, int64(1), state.FileReferences)
	require.Equal(t, int64(1), state.ThumbnailReferences)
	require.Equal(t, int64(2), state.BinaryContents)
}

func TestUploadAASXWithRejectedLastShellStoresNothing(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	before := readAtomicUploadPersistedState(t)

	status, body := uploadAASXPayload(t, buildAtomicUploadPackage(t, "Shell;1", false), "atomic-upload-invalid-shell.aasx")
	require.Equal(t, http.StatusBadRequest, status, string(body))
	requireAtomicUploadRejection(t, body, "-INVALIDIDSHORT", atomicUploadSecondShellID)

	requireAtomicUploadResourceStatus(t, http.StatusNotFound)
	require.Equal(t, before, readAtomicUploadPersistedState(t), "rejected upload must not leave resources, descriptors or binaries behind")
}

func TestUploadJSONWithRejectedLastShellStoresNothing(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	before := readAtomicUploadPersistedState(t)

	status, body := uploadAASXPayload(t, atomicUploadSpecification(t, "Shell;1"), "atomic-upload-invalid-shell.json")
	require.Equal(t, http.StatusBadRequest, status, string(body))
	requireAtomicUploadRejection(t, body, "-INVALIDIDSHORT", atomicUploadSecondShellID)

	requireAtomicUploadResourceStatus(t, http.StatusNotFound)
	require.Equal(t, before, readAtomicUploadPersistedState(t), "rejected upload must not leave resources or descriptors behind")
}

func TestUploadAASXWithRejectedLastAttachmentStoresNothing(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	before := readAtomicUploadPersistedState(t)

	status, body := uploadAASXPayload(t, buildAtomicUploadPackage(t, "Shell", true), "atomic-upload-invalid-attachment.aasx")
	require.Equal(t, http.StatusBadRequest, status, string(body))
	requireAtomicUploadRejection(t, body, "BINARYCONTENT-FILENAME-ESCAPE", atomicUploadSubmodelID)

	requireAtomicUploadResourceStatus(t, http.StatusNotFound)
	require.Equal(t, before, readAtomicUploadPersistedState(t), "rejected upload must not leave resources, descriptors, thumbnails or attachments behind")
}

func TestUploadAASXRejectedReplacementKeepsStoredEnvironment(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	status, body := uploadAASXPayload(t, buildAtomicUploadPackage(t, "Shell", false), "atomic-upload.aasx")
	require.Equal(t, http.StatusOK, status, string(body))
	before := readAtomicUploadPersistedState(t)
	storedDescription := readAtomicUploadResource(t, "/concept-descriptions/"+encodeAtomicUploadID(atomicUploadConceptDescriptionID))

	status, body = uploadAASXPayload(t, buildAtomicUploadPackage(t, "Shell;1", false), "atomic-upload-invalid-shell.aasx")
	require.Equal(t, http.StatusBadRequest, status, string(body))
	requireAtomicUploadRejection(t, body, "-INVALIDIDSHORT", atomicUploadSecondShellID)

	requireAtomicUploadResourceStatus(t, http.StatusOK)
	require.Equal(t, storedDescription, readAtomicUploadResource(t, "/concept-descriptions/"+encodeAtomicUploadID(atomicUploadConceptDescriptionID)))
	require.Equal(t, before, readAtomicUploadPersistedState(t), "rejected replacement must keep the stored environment unchanged")
}

func buildAtomicUploadPackage(t *testing.T, secondShellIDShort string, rejectLastAttachment bool) []byte {
	t.Helper()
	files := []fixtureEmbeddedFile{
		{FileName: "thumbnail.png", ContentType: "image/png", Content: atomicUploadThumbnail},
		{FileName: "manual.txt", ContentType: "text/plain", Content: []byte("atomic upload manual")},
	}
	specification := atomicUploadEnvironment(secondShellIDShort)
	if rejectLastAttachment {
		files = append(files, fixtureEmbeddedFile{FileName: atomicUploadRejectedAttachment, ContentType: "text/plain", Content: []byte("rejected")})
		submodel := specification["submodels"].([]any)[0].(map[string]any)
		submodel["submodelElements"] = append(submodel["submodelElements"].([]any), map[string]any{
			"modelType": "File", "idShort": "Rejected", "contentType": "text/plain", "value": "/aasx/files/" + atomicUploadRejectedAttachment,
		})
	}
	return buildUploadAASXFixture(t, marshalAtomicUploadEnvironment(t, specification), files...)
}

func atomicUploadSpecification(t *testing.T, secondShellIDShort string) []byte {
	t.Helper()
	return marshalAtomicUploadEnvironment(t, atomicUploadEnvironment(secondShellIDShort))
}

func marshalAtomicUploadEnvironment(t *testing.T, environment map[string]any) []byte {
	t.Helper()
	specification, err := json.Marshal(environment)
	require.NoError(t, err)
	return specification
}

func atomicUploadEnvironment(secondShellIDShort string) map[string]any {
	submodelReference := map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": atomicUploadSubmodelID}}}
	return map[string]any{
		"conceptDescriptions": []any{map[string]any{
			"modelType": "ConceptDescription", "id": atomicUploadConceptDescriptionID, "idShort": "AtomicDescription",
		}},
		"submodels": []any{map[string]any{
			"modelType": "Submodel", "id": atomicUploadSubmodelID, "idShort": "AtomicSubmodel",
			"submodelElements": []any{map[string]any{
				"modelType": "File", "idShort": "Manual", "contentType": "text/plain", "value": "/aasx/files/manual.txt",
			}},
		}},
		"assetAdministrationShells": []any{
			map[string]any{
				"modelType": "AssetAdministrationShell", "id": atomicUploadShellID, "idShort": "AtomicShell",
				"assetInformation": map[string]any{
					"assetKind": "Instance", "globalAssetId": atomicUploadShellID + ":asset",
					"defaultThumbnail": map[string]any{"path": "/aasx/files/thumbnail.png", "contentType": "image/png"},
				},
				"submodels": []any{submodelReference},
			},
			map[string]any{
				"modelType": "AssetAdministrationShell", "id": atomicUploadSecondShellID, "idShort": secondShellIDShort,
				"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": atomicUploadSecondShellID + ":asset"},
			},
		},
	}
}

func requireAtomicUploadRejection(t *testing.T, body []byte, cause string, failedResourceID string) {
	t.Helper()
	text := string(body)
	require.Contains(t, text, "AASENV-PROCESSENV-ROLLEDBACK", "response must state that nothing was stored")
	require.Contains(t, text, cause, "response must name the rejection cause")
	require.Contains(t, text, failedResourceID, "response must name the rejected resource")
}

func requireAtomicUploadResourceStatus(t *testing.T, expected int) {
	t.Helper()
	for _, resourcePath := range []string{
		"/concept-descriptions/" + encodeAtomicUploadID(atomicUploadConceptDescriptionID),
		"/submodels/" + encodeAtomicUploadID(atomicUploadSubmodelID),
		"/shells/" + encodeAtomicUploadID(atomicUploadShellID),
		"/submodel-descriptors/" + encodeAtomicUploadID(atomicUploadSubmodelID),
		"/shell-descriptors/" + encodeAtomicUploadID(atomicUploadShellID),
	} {
		requireUploadStatus(t, resourcePath, expected)
	}
}

func requireUploadStatus(t *testing.T, resourcePath string, expected int) {
	t.Helper()
	response := doHTTPIntegrationRequest(t, &http.Client{Timeout: 30 * time.Second}, mustNewRequest(t, http.MethodGet, aasEnvBaseURL+resourcePath))
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, expected, response.StatusCode, "GET %s: %s", resourcePath, string(responseBody))
}

func readAtomicUploadResource(t *testing.T, resourcePath string) string {
	t.Helper()
	response := doHTTPIntegrationRequest(t, &http.Client{Timeout: 30 * time.Second}, mustNewRequest(t, http.MethodGet, aasEnvBaseURL+resourcePath))
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))
	return string(responseBody)
}

func encodeAtomicUploadID(identifier string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(identifier))
}

func readAtomicUploadPersistedState(t *testing.T) atomicUploadPersistedState {
	t.Helper()
	db, err := sql.Open("pgx", uploadIntegrationDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	return atomicUploadPersistedState{
		LargeObjects:        countAtomicUploadRows(t, db, goqu.S("pg_catalog").Table("pg_largeobject_metadata")),
		BinaryContents:      countAtomicUploadRows(t, db, goqu.T("binary_content")),
		FileReferences:      countAtomicUploadRows(t, db, goqu.T("file_binary_reference")),
		ThumbnailReferences: countAtomicUploadRows(t, db, goqu.T("thumbnail_binary_reference")),
		SubmodelDescriptors: countAtomicUploadRows(t, db, goqu.T("submodel_descriptor")),
		ShellDescriptors:    countAtomicUploadRows(t, db, goqu.T("aas_descriptor")),
		ConceptDescriptions: countAtomicUploadRows(t, db, goqu.T("concept_description")),
		Submodels:           countAtomicUploadRows(t, db, goqu.T("submodel")),
		Shells:              countAtomicUploadRows(t, db, goqu.T("aas")),
	}
}

func countAtomicUploadRows(t *testing.T, db *sql.DB, table any) int64 {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From(table).Select(goqu.COUNT(goqu.Star())).ToSQL()
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.QueryRow(query, args...).Scan(&count))
	return count
}
