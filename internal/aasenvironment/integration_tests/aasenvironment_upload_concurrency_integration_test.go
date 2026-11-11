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
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const concurrentUploadRounds = 10

type concurrentUploadResult struct {
	status int
	body   string
	err    error
}

func TestConcurrentUploadsOfSharedSubmodelsInOppositeOrderSucceed(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	identifiers := make([]string, 0, 8)
	for index := range 8 {
		identifiers = append(identifiers, fmt.Sprintf("urn:basyx:integration:concurrent-upload:submodel:%d", index))
	}
	status, body := uploadAASXPayload(t, concurrentSubmodelEnvironment(t, identifiers, "initial"), "concurrent-initial.json")
	require.Equal(t, http.StatusOK, status, string(body))

	reversed := make([]string, len(identifiers))
	for index, identifier := range identifiers {
		reversed[len(identifiers)-1-index] = identifier
	}
	for round := range concurrentUploadRounds {
		value := fmt.Sprintf("round-%d", round)
		requireConcurrentUploadsSucceed(t,
			concurrentUpload{content: concurrentSubmodelEnvironment(t, identifiers, value+"-forward"), fileName: "concurrent-forward.json"},
			concurrentUpload{content: concurrentSubmodelEnvironment(t, reversed, value+"-reverse"), fileName: "concurrent-reverse.json"},
		)
	}
}

func TestConcurrentUploadsSharingAttachmentContentInOppositeOrderSucceed(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	first := fixtureEmbeddedFile{FileName: "first.txt", ContentType: "text/plain", Content: []byte("shared first attachment")}
	second := fixtureEmbeddedFile{FileName: "second.txt", ContentType: "text/plain", Content: []byte("shared second attachment")}

	for round := range concurrentUploadRounds {
		forwardID := fmt.Sprintf("urn:basyx:integration:concurrent-binary:forward:%d", round)
		reverseID := fmt.Sprintf("urn:basyx:integration:concurrent-binary:reverse:%d", round)
		requireConcurrentUploadsSucceed(t,
			concurrentUpload{content: concurrentAttachmentPackage(t, forwardID, first, second), fileName: "concurrent-forward.aasx"},
			concurrentUpload{content: concurrentAttachmentPackage(t, reverseID, second, first), fileName: "concurrent-reverse.aasx"},
		)
		for _, identifier := range []string{forwardID, reverseID} {
			for _, element := range []string{"First", "Second"} {
				requireUploadStatus(t, "/submodels/"+encodeAtomicUploadID(identifier)+"/submodel-elements/"+element+"/attachment", http.StatusOK)
			}
		}
	}
}

// Shell A references Submodels 1 and 4, Shell B references 2 and 3. Uploading
// [1, 2] and [3, 4] at once updates the descriptors A then B and B then A,
// although neither upload contains a Shell or shares a Submodel.
func TestConcurrentUploadsOfSubmodelsEmbeddedInTheSameShellDescriptorsInOppositeOrderSucceed(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	submodelIDs := concurrentIdentifiers("urn:basyx:integration:concurrent-descriptor:submodel", 4)
	shellA := concurrentShell("urn:basyx:integration:concurrent-descriptor:shell:a", submodelIDs[0], submodelIDs[3])
	shellB := concurrentShell("urn:basyx:integration:concurrent-descriptor:shell:b", submodelIDs[1], submodelIDs[2])
	initial := concurrentEnvironment(t, []any{shellA, shellB}, concurrentSubmodels(submodelIDs, "Initial", nil))
	status, body := uploadAASXPayload(t, initial, "concurrent-descriptor-initial.json")
	require.Equal(t, http.StatusOK, status, string(body))

	for round := range concurrentUploadRounds {
		idShort := fmt.Sprintf("Round%d", round)
		requireConcurrentUploadsSucceed(t,
			concurrentUpload{content: concurrentEnvironment(t, nil, concurrentSubmodels(submodelIDs[:2], idShort, nil)), fileName: "concurrent-descriptor-forward.json"},
			concurrentUpload{content: concurrentEnvironment(t, nil, concurrentSubmodels(submodelIDs[2:], idShort, nil)), fileName: "concurrent-descriptor-reverse.json"},
		)
	}
	for _, shellID := range []string{"urn:basyx:integration:concurrent-descriptor:shell:a", "urn:basyx:integration:concurrent-descriptor:shell:b"} {
		descriptor := readAtomicUploadResource(t, "/shell-descriptors/"+encodeAtomicUploadID(shellID))
		require.Contains(t, descriptor, fmt.Sprintf("Round%d", concurrentUploadRounds-1))
	}
}

// Submodels 1 and 4 reference content X, Submodels 2 and 3 content Y.
// Replacing [1, 2] and [3, 4] at once without their File elements releases
// X then Y and Y then X, although the uploads share no Submodel or binary.
func TestConcurrentUploadsReleasingSharedAttachmentContentInOppositeOrderSucceed(t *testing.T) {
	resetDatabaseForUploadIT(t, uploadIntegrationDSN)
	first := fixtureEmbeddedFile{FileName: "x.txt", ContentType: "text/plain", Content: []byte("released shared content x")}
	second := fixtureEmbeddedFile{FileName: "y.txt", ContentType: "text/plain", Content: []byte("released shared content y")}
	attachments := []string{first.FileName, second.FileName, second.FileName, first.FileName}

	for round := range concurrentUploadRounds {
		submodelIDs := concurrentIdentifiers(fmt.Sprintf("urn:basyx:integration:concurrent-release:%d:submodel", round), 4)
		initial := concurrentEnvironment(t, nil, concurrentSubmodels(submodelIDs, "Attached", attachments))
		status, body := uploadAASXPayload(t, buildUploadAASXFixture(t, initial, first, second), "concurrent-release-initial.aasx")
		require.Equal(t, http.StatusOK, status, string(body))

		requireConcurrentUploadsSucceed(t,
			concurrentUpload{content: concurrentEnvironment(t, nil, concurrentSubmodels(submodelIDs[:2], "Released", nil)), fileName: "concurrent-release-forward.json"},
			concurrentUpload{content: concurrentEnvironment(t, nil, concurrentSubmodels(submodelIDs[2:], "Released", nil)), fileName: "concurrent-release-reverse.json"},
		)
		for _, submodelID := range submodelIDs {
			requireUploadStatus(t, "/submodels/"+encodeAtomicUploadID(submodelID)+"/submodel-elements/Attachment", http.StatusNotFound)
		}
	}
	require.Zero(t, readAtomicUploadPersistedState(t).BinaryContents)
}

func concurrentIdentifiers(prefix string, count int) []string {
	identifiers := make([]string, 0, count)
	for index := range count {
		identifiers = append(identifiers, fmt.Sprintf("%s:%d", prefix, index+1))
	}
	return identifiers
}

func concurrentShell(identifier string, submodelIDs ...string) map[string]any {
	references := make([]any, 0, len(submodelIDs))
	for _, submodelID := range submodelIDs {
		references = append(references, map[string]any{
			"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": submodelID}},
		})
	}
	return map[string]any{
		"modelType": "AssetAdministrationShell", "id": identifier,
		"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": identifier + ":asset"},
		"submodels":        references,
	}
}

// concurrentSubmodels builds Submodels with 20 Properties each; attachments,
// when given, adds a File element referencing the package part of the same
// index.
func concurrentSubmodels(identifiers []string, idShort string, attachments []string) []any {
	submodels := make([]any, 0, len(identifiers))
	for index, identifier := range identifiers {
		elements := make([]any, 0, 21)
		for property := range 20 {
			elements = append(elements, map[string]any{
				"modelType": "Property", "idShort": fmt.Sprintf("Property%d", property), "valueType": "xs:string", "value": idShort,
			})
		}
		if index < len(attachments) {
			elements = append(elements, map[string]any{
				"modelType": "File", "idShort": "Attachment", "contentType": "text/plain", "value": "/aasx/files/" + attachments[index],
			})
		}
		submodels = append(submodels, map[string]any{
			"modelType": "Submodel", "id": identifier, "idShort": idShort, "submodelElements": elements,
		})
	}
	return submodels
}

func concurrentEnvironment(t *testing.T, shells []any, submodels []any) []byte {
	t.Helper()
	if shells == nil {
		shells = []any{}
	}
	specification, err := json.Marshal(map[string]any{"assetAdministrationShells": shells, "submodels": submodels, "conceptDescriptions": []any{}})
	require.NoError(t, err)
	return specification
}

type concurrentUpload struct {
	content  []byte
	fileName string
}

func requireConcurrentUploadsSucceed(t *testing.T, uploads ...concurrentUpload) {
	t.Helper()
	results := make([]concurrentUploadResult, len(uploads))
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for index, upload := range uploads {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			results[index] = postConcurrentUpload(upload)
		}()
	}
	close(start)
	waitGroup.Wait()
	for index, result := range results {
		require.NoError(t, result.err, uploads[index].fileName)
		require.Equal(t, http.StatusOK, result.status, "%s: %s", uploads[index].fileName, result.body)
	}
}

func postConcurrentUpload(upload concurrentUpload) concurrentUploadResult {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	filePart, err := writer.CreateFormFile("file", upload.fileName)
	if err != nil {
		return concurrentUploadResult{err: err}
	}
	if _, err = filePart.Write(upload.content); err != nil {
		return concurrentUploadResult{err: err}
	}
	if err = writer.Close(); err != nil {
		return concurrentUploadResult{err: err}
	}
	request, err := http.NewRequest(http.MethodPost, aasEnvBaseURL+"/upload", &payload)
	if err != nil {
		return concurrentUploadResult{err: err}
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	// #nosec G704 -- integration tests call controlled test-suite endpoints, not user-provided URLs.
	response, err := (&http.Client{Timeout: 120 * time.Second}).Do(request)
	if err != nil {
		return concurrentUploadResult{err: err}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	return concurrentUploadResult{status: response.StatusCode, body: string(body), err: err}
}

func concurrentSubmodelEnvironment(t *testing.T, identifiers []string, value string) []byte {
	t.Helper()
	submodels := make([]any, 0, len(identifiers))
	for _, identifier := range identifiers {
		elements := make([]any, 0, 20)
		for index := range 20 {
			elements = append(elements, map[string]any{
				"modelType": "Property", "idShort": fmt.Sprintf("Property%d", index), "valueType": "xs:string", "value": value,
			})
		}
		submodels = append(submodels, map[string]any{
			"modelType": "Submodel", "id": identifier, "idShort": "ConcurrentSubmodel", "submodelElements": elements,
		})
	}
	specification, err := json.Marshal(map[string]any{"assetAdministrationShells": []any{}, "submodels": submodels, "conceptDescriptions": []any{}})
	require.NoError(t, err)
	return specification
}

// concurrentAttachmentPackage builds a package whose File elements First and
// Second reference first.txt and second.txt; parts are stored in the given
// order, which decides the order in which their content is written.
func concurrentAttachmentPackage(t *testing.T, submodelID string, parts ...fixtureEmbeddedFile) []byte {
	t.Helper()
	fileElement := func(idShort string, fileName string) map[string]any {
		return map[string]any{"modelType": "File", "idShort": idShort, "contentType": "text/plain", "value": "/aasx/files/" + fileName}
	}
	specification, err := json.Marshal(map[string]any{
		"assetAdministrationShells": []any{},
		"submodels": []any{map[string]any{
			"modelType": "Submodel", "id": submodelID, "idShort": "ConcurrentBinarySubmodel",
			"submodelElements": []any{fileElement("First", "first.txt"), fileElement("Second", "second.txt")},
		}},
		"conceptDescriptions": []any{},
	})
	require.NoError(t, err)
	return buildUploadAASXFixture(t, specification, parts...)
}
