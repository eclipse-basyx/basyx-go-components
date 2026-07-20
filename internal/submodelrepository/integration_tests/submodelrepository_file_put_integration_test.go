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

//nolint:all
package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

// TestPutFileSubmodelElementWithoutValue reproduces the panic and NULL-scan
// failures that occurred when a File submodel element was PUT without the
// optional value field (see PostgreSQLFileHandler.Update).
func TestPutFileSubmodelElementWithoutValue(t *testing.T) {
	baseURL := submodelRepositoryBaseURL
	submodelID := fmt.Sprintf("urn:basyx:integration:file-put-no-value-%d", time.Now().UnixNano())
	submodelIDEncoded := base64.RawURLEncoding.EncodeToString([]byte(submodelID))

	payload := map[string]any{
		"id":        submodelID,
		"idShort":   "FilePutSubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":     "DocFile",
				"modelType":   "File",
				"contentType": "application/pdf",
				"value":       "/aasx/files/doc.pdf",
			},
			map[string]any{
				"idShort":     "EmptyFile",
				"modelType":   "File",
				"contentType": "application/pdf",
			},
		},
	}

	statusCode, body, err := requestJSON(http.MethodPost, baseURL+"/submodels", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

	t.Cleanup(func() {
		_, _, _ = requestJSON(http.MethodDelete, baseURL+"/submodels/"+submodelIDEncoded, nil)
	})

	t.Run("PutWithoutValueOnFileWithStoredValue", func(t *testing.T) {
		status, responseBody, requestErr := requestJSON(http.MethodPut, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/DocFile", map[string]any{
			"idShort":     "DocFile",
			"modelType":   "File",
			"contentType": "application/pdf",
		})
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusNoContent, status, "response=%s", string(responseBody))

		status, responseBody, requestErr = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/DocFile", nil)
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))

		var element map[string]any
		require.NoError(t, json.Unmarshal(responseBody, &element), "response=%s", string(responseBody))
		require.Equal(t, "File", element["modelType"])
		if rawValue, hasValue := element["value"]; hasValue && rawValue != nil {
			require.Equal(t, "", rawValue, "file value must be cleared after PUT without value")
		}
	})

	t.Run("PutWithoutValueOnFileWithoutStoredValue", func(t *testing.T) {
		status, responseBody, requestErr := requestJSON(http.MethodPut, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/EmptyFile", map[string]any{
			"idShort":     "EmptyFile",
			"modelType":   "File",
			"contentType": "text/plain",
		})
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusNoContent, status, "response=%s", string(responseBody))
	})

	t.Run("PutWithValueStillPersistsValue", func(t *testing.T) {
		status, responseBody, requestErr := requestJSON(http.MethodPut, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/EmptyFile", map[string]any{
			"idShort":     "EmptyFile",
			"modelType":   "File",
			"contentType": "text/plain",
			"value":       "/aasx/files/readme.txt",
		})
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusNoContent, status, "response=%s", string(responseBody))

		status, responseBody, requestErr = requestJSON(http.MethodGet, baseURL+"/submodels/"+submodelIDEncoded+"/submodel-elements/EmptyFile", nil)
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusOK, status, "response=%s", string(responseBody))

		var element map[string]any
		require.NoError(t, json.Unmarshal(responseBody, &element), "response=%s", string(responseBody))
		require.Equal(t, "/aasx/files/readme.txt", element["value"], "response=%s", string(responseBody))
	})
}

func TestFullSubmodelPutPreservesOwnedManagedAttachment(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:full-put-file-%d", time.Now().UnixNano())
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := submodelRepositoryBaseURL + "/submodels/" + encodedID
	payload := fileReplacementSubmodelPayload(submodelID, []string{"Document"})
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, endpoint, nil) })

	attachmentEndpoint := endpoint + "/submodel-elements/Document/attachment"
	filePath := createTemporaryBinaryTestFile(t, "full-put-file", []byte("managed-file-content"))
	status, err = uploadFileAttachment(attachmentEndpoint, filePath, "document.txt")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	status, body, err = requestJSON(http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	var storedSubmodel map[string]any
	require.NoError(t, json.Unmarshal(body, &storedSubmodel))
	managedPath := storedSubmodel["submodelElements"].([]any)[0].(map[string]any)["value"]

	status, body, err = requestJSON(http.MethodPut, endpoint, storedSubmodel)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	content, _, downloadStatus, err := downloadFileAttachment(attachmentEndpoint)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, downloadStatus)
	require.Equal(t, []byte("managed-file-content"), content)
	require.Equal(t, managedPath, getFileElementValue(t, endpoint+"/submodel-elements/Document"))
}

func TestConcurrentSubmodelDeletionCleansSharedCanonicalFilesWithoutDeadlock(t *testing.T) {
	baseline := binaryContentRowCount(t)
	leftID := fmt.Sprintf("urn:basyx:integration:delete-left-%d", time.Now().UnixNano())
	rightID := fmt.Sprintf("urn:basyx:integration:delete-right-%d", time.Now().UnixNano())
	leftEndpoint := createSubmodelWithManagedFiles(t, leftID, []string{"First", "Second"}, [][]byte{[]byte("shared-x"), []byte("shared-y")})
	rightEndpoint := createSubmodelWithManagedFiles(t, rightID, []string{"First", "Second"}, [][]byte{[]byte("shared-y"), []byte("shared-x")})

	start := make(chan struct{})
	statuses := make(chan int, 2)
	errors := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, endpoint := range []string{leftEndpoint, rightEndpoint} {
		waitGroup.Add(1)
		go func(deleteEndpoint string) {
			defer waitGroup.Done()
			<-start
			status, _, err := requestJSON(http.MethodDelete, deleteEndpoint, nil)
			statuses <- status
			errors <- err
		}(endpoint)
	}
	close(start)
	waitGroup.Wait()
	close(statuses)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	for status := range statuses {
		require.Equal(t, http.StatusNoContent, status)
	}
	require.Equal(t, baseline, binaryContentRowCount(t))
}

func createSubmodelWithManagedFiles(t *testing.T, submodelID string, fileNames []string, contents [][]byte) string {
	t.Helper()
	endpoint := submodelRepositoryBaseURL + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", fileReplacementSubmodelPayload(submodelID, fileNames))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, endpoint, nil) })
	for index, fileName := range fileNames {
		filePath := createTemporaryBinaryTestFile(t, "shared-file", contents[index])
		status, err = uploadFileAttachment(endpoint+"/submodel-elements/"+fileName+"/attachment", filePath, fileName+".bin")
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, status)
	}
	return endpoint
}

func fileReplacementSubmodelPayload(submodelID string, fileNames []string) map[string]any {
	elements := make([]any, 0, len(fileNames))
	for _, fileName := range fileNames {
		elements = append(elements, map[string]any{
			"idShort": fileName, "modelType": "File", "contentType": "application/octet-stream",
		})
	}
	return map[string]any{
		"id": submodelID, "idShort": "ManagedFiles", "kind": "Instance", "modelType": "Submodel", "submodelElements": elements,
	}
}

func binaryContentRowCount(t *testing.T) int64 {
	t.Helper()
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.From("binary_content").Select(goqu.COUNT("*")).ToSQL()
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}
