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
