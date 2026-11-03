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
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
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

func TestUploadFileAttachmentInIndexedSubmodelElementLists(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:indexed-list-file-%d", time.Now().UnixNano())
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := submodelRepositoryBaseURL + "/submodels/" + encodedID
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", indexedListFileSubmodelPayload(submodelID))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, endpoint, nil) })

	idShortPath := "Model3D[105].File.FileVersion[0].DigitalFile"
	elementEndpoint := endpoint + "/submodel-elements/" + url.PathEscape(idShortPath)
	attachmentEndpoint := endpoint + "/submodel-elements/" + url.PathEscape(idShortPath) + "/attachment"
	status, body, err = requestJSON(http.MethodGet, elementEndpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	var element map[string]any
	require.NoError(t, json.Unmarshal(body, &element))
	require.Equal(t, "File", element["modelType"])
	require.Equal(t, "/aasx/files/model.step", element["value"])
	issuePath := "Model3D[0].File.FileVersion[0].DigitalFile"
	status, body, err = requestJSON(http.MethodGet, endpoint+"/submodel-elements/"+url.PathEscape(issuePath), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	require.NoError(t, json.Unmarshal(body, &element))
	require.Equal(t, "File", element["modelType"])
	require.Equal(t, "/aasx/files/model.step", element["value"])

	payload := []byte("indexed list attachment")
	filePath := createTemporaryBinaryTestFile(t, "model.step", payload)
	status, err = uploadFileAttachment(attachmentEndpoint, filePath, "model.step")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	status, body, err = requestJSON(http.MethodGet, elementEndpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	require.NoError(t, json.Unmarshal(body, &element))
	require.Equal(t, "File", element["modelType"])

	content, _, status, err := downloadFileAttachment(attachmentEndpoint)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, payload, content)
}

func indexedListFileSubmodelPayload(submodelID string) map[string]any {
	modelEntries := make([]any, 106)
	for index := range modelEntries {
		modelEntries[index] = indexedListFileEntry()
	}
	return map[string]any{
		"id": submodelID, "idShort": "IndexedListFiles", "kind": "Instance", "modelType": "Submodel",
		"submodelElements": []any{map[string]any{
			"idShort": "Model3D", "modelType": "SubmodelElementList", "orderRelevant": true,
			"typeValueListElement": "SubmodelElementCollection", "value": modelEntries,
		}},
	}
}

func indexedListFileEntry() map[string]any {
	return map[string]any{
		"modelType": "SubmodelElementCollection",
		"value": []any{map[string]any{
			"idShort": "File", "modelType": "SubmodelElementCollection",
			"value": []any{map[string]any{
				"idShort": "FileVersion", "modelType": "SubmodelElementList", "orderRelevant": true,
				"typeValueListElement": "SubmodelElementCollection",
				"value": []any{map[string]any{
					"modelType": "SubmodelElementCollection",
					"value": []any{map[string]any{
						"idShort": "DigitalFile", "modelType": "File", "contentType": "model/step", "value": "/aasx/files/model.step",
					}},
				}},
			}},
		}},
	}
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

func TestDownloadManagedAttachmentWithMissingFileName(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:managed-file-name-%d", time.Now().UnixNano())
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := submodelRepositoryBaseURL + "/submodels/" + encodedID
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", fileReplacementSubmodelPayload(submodelID, []string{"Document", "Empty"}))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, endpoint, nil) })

	attachmentEndpoint := endpoint + "/submodel-elements/Document/attachment"
	emptyAttachmentEndpoint := endpoint + "/submodel-elements/Empty/attachment"
	status, body, err = requestJSON(http.MethodGet, emptyAttachmentEndpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status, "response=%s", string(body))

	payload := []byte("managed attachment content")
	filePath := createTemporaryBinaryTestFile(t, "manual.pdf", payload)
	status, err = uploadFileAttachment(attachmentEndpoint, filePath, "manual.pdf")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)

	setManagedAttachmentFileName(t, submodelID, "Document", nil)
	content, contentType, downloadStatus, err := downloadFileAttachment(attachmentEndpoint)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, downloadStatus)
	require.Equal(t, "application/pdf", contentType)
	require.Equal(t, payload, content)
	assertAttachmentFileName(t, attachmentEndpoint, "manual.pdf")

	setManagedAttachmentFileName(t, submodelID, "Document", "")
	content, contentType, downloadStatus, err = downloadFileAttachment(attachmentEndpoint)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, downloadStatus)
	require.Equal(t, "application/pdf", contentType)
	require.Equal(t, payload, content)
	assertAttachmentFileName(t, attachmentEndpoint, "manual.pdf")
}

func assertAttachmentFileName(t *testing.T, endpoint string, fileName string) {
	t.Helper()
	status, body, headers, err := requestJSONWithHeaders(http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	require.Contains(t, headers.Get("Content-Disposition"), fileName)
}

func setManagedAttachmentFileName(t *testing.T, submodelID string, idShortPath string, fileName any) {
	t.Helper()
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	elementIDQuery := goqu.From(goqu.T("submodel_element").As("sme")).
		Join(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("sme.submodel_id")))).
		Select(goqu.I("sme.id")).
		Where(goqu.I("sm.submodel_identifier").Eq(submodelID), goqu.I("sme.idshort_path").Eq(idShortPath))
	query, args, err := goqu.Update("file_element").Set(goqu.Record{"file_name": fileName}).
		Where(goqu.C("id").Eq(elementIDQuery)).ToSQL()
	require.NoError(t, err)
	result, err := db.Exec(query, args...)
	require.NoError(t, err)
	updated, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)
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

// TestUploadFileAttachmentWithoutFileName covers uploads whose file part has no
// filename (eclipse-basyx/basyx-go-components#813): the file name is optional
// in the V3.2 PutFileByPath request body.
func TestUploadFileAttachmentWithoutFileName(t *testing.T) {
	submodelID := fmt.Sprintf("urn:basyx:integration:nameless-upload-%d", time.Now().UnixNano())
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := submodelRepositoryBaseURL + "/submodels/" + encodedID
	status, body, err := requestJSON(http.MethodPost, submodelRepositoryBaseURL+"/submodels", map[string]any{
		"id":        submodelID,
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{"idShort": "manual", "modelType": "File", "contentType": "text/plain"},
			map[string]any{"idShort": "guide", "modelType": "File", "contentType": "text/plain"},
			map[string]any{
				"idShort":              "Images",
				"modelType":            "SubmodelElementList",
				"typeValueListElement": "File",
				"value":                []any{map[string]any{"modelType": "File", "contentType": "image/png"}},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _, _ = requestJSON(http.MethodDelete, endpoint, nil) })

	t.Run("FirstUploadIsNamedAfterElement", func(t *testing.T) {
		payload := []byte("Hello, manual!")
		attachmentEndpoint := endpoint + "/submodel-elements/manual/attachment"
		require.Equal(t, http.StatusNoContent, putRawMultipartAttachment(t, attachmentEndpoint, `form-data; name="file"`, "text/plain", payload))

		assertManagedFileElement(t, endpoint+"/submodel-elements/manual", "manual.txt", "text/plain")
		content, _, downloadStatus, downloadErr := downloadFileAttachment(attachmentEndpoint)
		require.NoError(t, downloadErr)
		require.Equal(t, http.StatusOK, downloadStatus)
		require.Equal(t, payload, content)
		assertAttachmentFileName(t, attachmentEndpoint, "manual.txt")
	})

	t.Run("ListItemWithoutIdShortIsNamedFile", func(t *testing.T) {
		attachmentEndpoint := endpoint + "/submodel-elements/" + url.PathEscape("Images[0]") + "/attachment"
		png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")
		require.Equal(t, http.StatusNoContent, putRawMultipartAttachment(t, attachmentEndpoint, `form-data; name="file"`, "image/png", png))

		assertManagedFileElement(t, endpoint+"/submodel-elements/"+url.PathEscape("Images[0]"), "file.png", "image/png")
	})

	t.Run("PartFileNameIsUsedAndFileNameFieldIgnored", func(t *testing.T) {
		attachmentEndpoint := endpoint + "/submodel-elements/guide/attachment"
		status := putRawMultipartAttachmentWithFileNameField(t, attachmentEndpoint, "ignored.txt", `form-data; name="file"; filename="handbook.txt"`, "text/plain", []byte("guide"))
		require.Equal(t, http.StatusNoContent, status)

		assertManagedFileElement(t, endpoint+"/submodel-elements/guide", "handbook.txt", "text/plain")
	})
}

func assertManagedFileElement(t *testing.T, elementEndpoint string, fileName string, contentType string) {
	t.Helper()
	status, body, err := requestJSON(http.MethodGet, elementEndpoint, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	var element map[string]any
	require.NoError(t, json.Unmarshal(body, &element))
	require.Regexp(t, `^/aasx/files/[A-Za-z0-9_-]{32}/`+regexp.QuoteMeta(fileName)+`$`, element["value"])
	require.Equal(t, contentType, element["contentType"])
}

func putRawMultipartAttachment(t *testing.T, endpoint string, contentDisposition string, contentType string, payload []byte) int {
	t.Helper()
	return putRawMultipartAttachmentWithFileNameField(t, endpoint, "", contentDisposition, contentType, payload)
}

func putRawMultipartAttachmentWithFileNameField(t *testing.T, endpoint string, fileNameField string, contentDisposition string, contentType string, payload []byte) int {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if fileNameField != "" {
		require.NoError(t, writer.WriteField("fileName", fileNameField))
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", contentDisposition)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint, &body)
	require.NoError(t, err)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		t.Logf("attachment upload response: %s", string(responseBody))
	}
	return response.StatusCode
}
