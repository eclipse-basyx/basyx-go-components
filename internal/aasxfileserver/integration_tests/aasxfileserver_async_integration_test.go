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
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
	"github.com/stretchr/testify/require"
)

const (
	ssp001Profile = "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-001"
	ssp002Profile = "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-002"
	validAASXPath = "../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx"
	asyncDeadline = 45 * time.Second
)

type operationHandle struct {
	HandleID string `json:"handleId"`
}

type baseOperationResult struct {
	ExecutionState string            `json:"executionState"`
	Success        bool              `json:"success"`
	Messages       []json.RawMessage `json:"messages,omitempty"`
}

type asyncResponse struct {
	status int
	header http.Header
	body   []byte
}

func TestAsyncUploadCompletesAfterRequestCleanupAndAcrossReplicas(t *testing.T) {
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)

	accepted := postAsyncFile(t, baseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Completed", result.ExecutionState)
	require.True(t, result.Success)

	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	t.Cleanup(func() {
		response := doAsyncRequest(t, http.MethodDelete, replicaBURL+"/packages/"+url.PathEscape(created.PackageId), nil, "")
		require.Equal(t, http.StatusNoContent, response.status)
	})

	download := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages/"+url.PathEscape(created.PackageId), nil, "")
	require.Equal(t, http.StatusOK, download.status)
	expected, err := os.ReadFile(filepath.Clean(validAASXPath))
	require.NoError(t, err)
	require.Equal(t, expected, download.body)
	require.Equal(t, filepath.Base(validAASXPath), created.FileName)
}

func TestAsyncStatusAndResultContracts(t *testing.T) {
	t.Run("unknown handle", func(t *testing.T) {
		for _, endpoint := range []string{"status", "result"} {
			response := doAsyncRequest(t, http.MethodGet, baseURL+"/packages-async/"+endpoint+"/unknown-handle", nil, "")
			require.Equalf(t, http.StatusNotFound, response.status, "%s response: %s", endpoint, response.body)
		}
	})

	t.Run("terminal status redirects without client auto follow", func(t *testing.T) {
		accepted := postAsyncFile(t, baseURL, validAASXPath)
		handle := assertAcceptedOperation(t, accepted, baseURL)
		statusResponse := awaitTerminalStatus(t, replicaBURL, handle)
		assertHandleLocation(t, statusResponse.header.Get("Location"), replicaBURL, "/packages-async/result/", handle)

		resultResponse := doAsyncRequest(t, http.MethodGet, resolveLocation(t, replicaBURL, statusResponse.header.Get("Location")), nil, "")
		require.Equal(t, http.StatusOK, resultResponse.status)
		require.Equal(t, "application/json", mediaType(t, resultResponse.header.Get("Content-Type")))
		result := decodeBaseOperationResult(t, resultResponse.body)
		require.Contains(t, []string{"Completed", "Failed", "Canceled", "Timeout"}, result.ExecutionState)
	})
}

func TestAsyncUploadValidationMatchesSynchronousEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		body       func(t *testing.T) (io.Reader, string)
		wantStatus int
	}{
		{
			name: "non multipart request",
			body: func(_ *testing.T) (io.Reader, string) {
				return strings.NewReader(`{"file":"not multipart"}`), "application/json"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing file part",
			body: func(t *testing.T) (io.Reader, string) {
				var payload bytes.Buffer
				writer := multipart.NewWriter(&payload)
				require.NoError(t, writer.WriteField("aasIds", "urn:example:aas"))
				require.NoError(t, writer.Close())
				return &payload, writer.FormDataContentType()
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "empty multipart body",
			body: func(_ *testing.T) (io.Reader, string) {
				return strings.NewReader(""), "multipart/form-data; boundary=empty"
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			asyncBody, asyncContentType := testCase.body(t)
			async := doAsyncRequest(t, http.MethodPost, baseURL+"/packages-async", asyncBody, asyncContentType)
			require.Equalf(t, testCase.wantStatus, async.status, "async response: %s", async.body)
			require.NotEqual(t, http.StatusAccepted, async.status)

			syncBody, syncContentType := testCase.body(t)
			synchronous := doAsyncRequest(t, http.MethodPost, baseURL+"/packages", syncBody, syncContentType)
			require.Equalf(t, synchronous.status, async.status, "SSP-002 validation drifted from SSP-001: sync=%s async=%s", synchronous.body, async.body)
		})
	}
}

func TestAsyncUploadRejectsOversizedFileBeforeAcceptance(t *testing.T) {
	oversized := io.LimitReader(zeroReader{}, 1025)
	response := postAsyncReader(t, limitedBaseURL, "oversized.aasx", oversized)
	require.Equalf(t, http.StatusRequestEntityTooLarge, response.status, "response: %s", response.body)
	require.NotEqual(t, http.StatusAccepted, response.status)
}

func TestAcceptedAsyncUploadReportsBackgroundPackageFailureAndCleansStaging(t *testing.T) {
	db := openIntegrationDatabase(t)
	largeObjectsBefore := countLargeObjects(t, db)
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)

	accepted := postAsyncReader(t, baseURL, "invalid.aasx", strings.NewReader("not an OPC package"))
	handle := assertAcceptedOperation(t, accepted, baseURL)
	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Failed", result.ExecutionState)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Messages)

	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	require.ElementsMatch(t, collectPackageIDs(packagesBefore.Result), collectPackageIDs(packagesAfter.Result))
	require.Eventually(t, func() bool { return countLargeObjects(t, db) == largeObjectsBefore }, 5*time.Second, 50*time.Millisecond)
	retained := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages-async/result/"+url.PathEscape(handle), nil, "")
	require.Equal(t, http.StatusOK, retained.status)
	require.Equal(t, "Failed", decodeBaseOperationResult(t, retained.body).ExecutionState)

	expireAsyncHandle(t, db, handle)
	expired := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages-async/result/"+url.PathEscape(handle), nil, "")
	require.Equal(t, http.StatusNotFound, expired.status)
}

func TestAsyncLocationIncludesConfiguredContextPath(t *testing.T) {
	accepted := postAsyncFile(t, contextBaseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, contextBaseURL)
	location := accepted.header.Get("Location")
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	require.Equal(t, "/external/aasx/packages-async/status/"+url.PathEscape(handle), parsed.Path)
	require.NotContains(t, location, "aasx_fileserver_context_it")
}

func TestAsyncTerminalRetentionStartsAtCompletionAndExpiresThroughCleanup(t *testing.T) {
	accepted := postAsyncFile(t, baseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	_ = awaitAsyncResult(t, replicaBURL, handle)
	retained := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages-async/result/"+url.PathEscape(handle), nil, "")
	require.Equal(t, http.StatusOK, retained.status)
	require.Equal(t, "Completed", decodeBaseOperationResult(t, retained.body).ExecutionState)

	db := openIntegrationDatabase(t)
	var terminalAt, expiresAt time.Time
	query, args, err := goqu.Dialect("postgres").From("async_job").
		Select("terminal_at", "expires_at").Where(goqu.C("handle_id").Eq(handle)).ToSQL()
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&terminalAt, &expiresAt))
	require.True(t, expiresAt.After(terminalAt), "retention must start from the terminal transition")

	expireQuery, expireArgs, err := goqu.Dialect("postgres").Update("async_job").
		Set(goqu.Record{"expires_at": time.Now().UTC().Add(-time.Second)}).
		Where(goqu.C("handle_id").Eq(handle)).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), expireQuery, expireArgs...)
	require.NoError(t, err)

	for _, endpoint := range []string{"status", "result"} {
		response := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages-async/"+endpoint+"/"+url.PathEscape(handle), nil, "")
		require.Equal(t, http.StatusNotFound, response.status)
	}
}

func TestRunningAsyncHandleDoesNotExpireAndResultIsNotAvailable(t *testing.T) {
	accepted := postAsyncFile(t, baseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	_ = awaitAsyncResult(t, replicaBURL, handle)

	db := openIntegrationDatabase(t)
	query, args, err := goqu.Dialect("postgres").Update("async_job").Set(goqu.Record{
		"execution_state":  "Running",
		"terminal_at":      nil,
		"expires_at":       time.Now().UTC().Add(-time.Second),
		"lease_expires_at": time.Now().UTC().Add(time.Minute),
	}).Where(goqu.C("handle_id").Eq(handle)).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)

	statusResponse := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages-async/status/"+url.PathEscape(handle), nil, "")
	require.Equal(t, http.StatusOK, statusResponse.status)
	running := decodeBaseOperationResult(t, statusResponse.body)
	require.Equal(t, "Running", running.ExecutionState)
	require.True(t, running.Success)

	resultResponse := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages-async/result/"+url.PathEscape(handle), nil, "")
	require.Equal(t, http.StatusBadRequest, resultResponse.status)
}

func TestSuccessfulAsyncUploadRetainsOneReferencedLargeObjectUntilPackageDeletion(t *testing.T) {
	db := openIntegrationDatabase(t)
	largeObjectsBefore := countLargeObjects(t, db)
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)

	accepted := postAsyncFile(t, baseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	result := awaitAsyncResult(t, replicaBURL, handle)
	require.True(t, result.Success)

	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	require.Equal(t, largeObjectsBefore+1, countLargeObjects(t, db), "successful staging left a duplicate large object")

	deleted := doAsyncRequest(t, http.MethodDelete, replicaBURL+"/packages/"+url.PathEscape(created.PackageId), nil, "")
	require.Equal(t, http.StatusNoContent, deleted.status)
	require.Eventually(t, func() bool { return countLargeObjects(t, db) == largeObjectsBefore }, 5*time.Second, 50*time.Millisecond)
}

func TestInterruptedAsyncWorkerRecoversAsFailureAcrossReplicas(t *testing.T) {
	if composeProject == "" {
		t.Skip("worker interruption requires the integration suite's managed Compose project")
	}

	db := openIntegrationDatabase(t)
	accepted := postAsyncFile(t, baseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, baseURL)

	stopComposeService(t, "aasx_fileserver_it")
	t.Cleanup(func() { startComposeService(t, "aasx_fileserver_it", baseURL+"/health") })
	setHandleToAbandonedRunningState(t, db, handle)

	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Failed", result.ExecutionState)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Messages)
}

func TestDescriptionAdvertisesSynchronousAndAsynchronousProfiles(t *testing.T) {
	response := doAsyncRequest(t, http.MethodGet, baseURL+"/description", nil, "")
	require.Equal(t, http.StatusOK, response.status)
	var description openapi.ServiceDescription
	require.NoError(t, json.Unmarshal(response.body, &description))
	require.Contains(t, description.Profiles, ssp001Profile)
	require.Contains(t, description.Profiles, ssp002Profile)
}

func assertAcceptedOperation(t *testing.T, response asyncResponse, requestBaseURL string) string {
	t.Helper()
	require.Equalf(t, http.StatusAccepted, response.status, "response: %s", response.body)
	require.Equal(t, "application/json", mediaType(t, response.header.Get("Content-Type")))

	var handle operationHandle
	require.NoError(t, json.Unmarshal(response.body, &handle))
	require.NotEmpty(t, handle.HandleID)
	require.LessOrEqual(t, len(handle.HandleID), 128)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response.body, &raw))
	require.Contains(t, raw, "handleId")
	require.NotContains(t, raw, "packageId", "POST /packages-async must not return the synchronous package result")
	require.NotContains(t, raw, "executionState", "POST /packages-async must return OperationHandle, not a final result")
	assertHandleLocation(t, response.header.Get("Location"), requestBaseURL, "/packages-async/status/", handle.HandleID)
	return handle.HandleID
}

func awaitAsyncResult(t *testing.T, pollingBaseURL string, handle string) baseOperationResult {
	t.Helper()
	terminal := awaitTerminalStatus(t, pollingBaseURL, handle)
	resultURL := resolveLocation(t, pollingBaseURL, terminal.header.Get("Location"))
	resultResponse := doAsyncRequest(t, http.MethodGet, resultURL, nil, "")
	require.Equalf(t, http.StatusOK, resultResponse.status, "result response: %s", resultResponse.body)
	require.Equal(t, "application/json", mediaType(t, resultResponse.header.Get("Content-Type")))
	return decodeBaseOperationResult(t, resultResponse.body)
}

func awaitTerminalStatus(t *testing.T, pollingBaseURL string, handle string) asyncResponse {
	t.Helper()
	deadline := time.Now().Add(asyncDeadline)
	for time.Now().Before(deadline) {
		response := doAsyncRequest(t, http.MethodGet, pollingBaseURL+"/packages-async/status/"+url.PathEscape(handle), nil, "")
		switch response.status {
		case http.StatusOK:
			running := decodeBaseOperationResult(t, response.body)
			require.Equal(t, "Running", running.ExecutionState)
			require.True(t, running.Success)
		case http.StatusFound:
			assertHandleLocation(t, response.header.Get("Location"), pollingBaseURL, "/packages-async/result/", handle)
			return response
		default:
			t.Fatalf("status polling returned %d: %s", response.status, response.body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("operation %q did not become terminal within %s", handle, asyncDeadline)
	return asyncResponse{}
}

func postAsyncFile(t *testing.T, serviceURL string, path string) asyncResponse {
	t.Helper()
	file, err := os.Open(filepath.Clean(path))
	require.NoError(t, err)
	response := postAsyncReader(t, serviceURL, filepath.Base(path), file)
	require.NoError(t, file.Close())
	return response
}

func postAsyncReader(t *testing.T, serviceURL string, filename string, content io.Reader) asyncResponse {
	t.Helper()
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	writeDone := make(chan error, 1)
	go func() {
		part, err := multipartWriter.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, content)
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
		writeDone <- err
	}()
	response := doAsyncRequest(t, http.MethodPost, serviceURL+"/packages-async", reader, multipartWriter.FormDataContentType())
	writeErr := <-writeDone
	if response.status == http.StatusAccepted {
		require.NoError(t, writeErr)
	}
	return response
}

func doAsyncRequest(t *testing.T, method string, endpoint string, body io.Reader, contentType string) asyncResponse {
	t.Helper()
	response, err := executeAsyncRequest(method, endpoint, body, contentType)
	require.NoError(t, err)
	return response
}

func executeAsyncRequest(method string, endpoint string, body io.Reader, contentType string) (asyncResponse, error) {
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return asyncResponse{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	client := &http.Client{
		Timeout: asyncDeadline,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req) // #nosec G704 -- endpoints are fixed local integration-test URLs.
	if err != nil {
		return asyncResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return asyncResponse{}, err
	}
	return asyncResponse{status: resp.StatusCode, header: resp.Header.Clone(), body: responseBody}, nil
}

func listPackagesFrom(t *testing.T, serviceURL string) (openapi.GetPackageDescriptionsResult, int) {
	t.Helper()
	response := doAsyncRequest(t, http.MethodGet, serviceURL+"/packages", nil, "")
	var result openapi.GetPackageDescriptionsResult
	require.NoError(t, json.Unmarshal(response.body, &result))
	return result, response.status
}

func packageDifference(t *testing.T, before []openapi.PackageDescription, after []openapi.PackageDescription) openapi.PackageDescription {
	t.Helper()
	known := make(map[string]struct{}, len(before))
	for _, item := range before {
		known[item.PackageId] = struct{}{}
	}
	created := make([]openapi.PackageDescription, 0, 1)
	for _, item := range after {
		if _, found := known[item.PackageId]; !found {
			created = append(created, item)
		}
	}
	require.Len(t, created, 1)
	return created[0]
}

func decodeBaseOperationResult(t *testing.T, body []byte) baseOperationResult {
	t.Helper()
	var result baseOperationResult
	require.NoError(t, json.Unmarshal(body, &result))
	require.NotEmpty(t, result.ExecutionState)
	return result
}

func assertHandleLocation(t *testing.T, location string, requestBaseURL string, pathPrefix string, handle string) {
	t.Helper()
	require.NotEmpty(t, location)
	resolved, err := url.Parse(resolveLocation(t, requestBaseURL, location))
	require.NoError(t, err)
	base, err := url.Parse(requestBaseURL)
	require.NoError(t, err)
	expectedPath := strings.TrimSuffix(base.Path, "/") + pathPrefix + url.PathEscape(handle)
	require.Equal(t, expectedPath, resolved.EscapedPath())
	require.Equal(t, handle, strings.TrimPrefix(resolved.Path, strings.TrimSuffix(base.Path, "/")+pathPrefix))
	if resolved.IsAbs() {
		require.Equal(t, base.Host, resolved.Host, "Location exposed an internal or unrelated host")
	}
}

func resolveLocation(t *testing.T, serviceURL string, location string) string {
	t.Helper()
	base, err := url.Parse(strings.TrimSuffix(serviceURL, "/") + "/")
	require.NoError(t, err)
	reference, err := url.Parse(location)
	require.NoError(t, err)
	return base.ResolveReference(reference).String()
}

func mediaType(t *testing.T, contentType string) string {
	t.Helper()
	return strings.TrimSpace(strings.Split(contentType, ";")[0])
}

func openIntegrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := common.NewDatabaseConnection(databaseDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func expireAsyncHandle(t *testing.T, db *sql.DB, handle string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Update("async_job").
		Set(goqu.Record{"expires_at": time.Now().UTC().Add(-time.Second)}).
		Where(goqu.C("handle_id").Eq(handle)).ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func setHandleToAbandonedRunningState(t *testing.T, db *sql.DB, handle string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Update("async_job").Set(goqu.Record{
		"execution_state":  "Running",
		"worker_id":        "interrupted-worker",
		"terminal_at":      nil,
		"expires_at":       nil,
		"lease_expires_at": time.Now().UTC().Add(-time.Second),
		"result_payload":   nil,
		"error_status":     nil,
		"error_payload":    nil,
	}).Where(goqu.C("handle_id").Eq(handle)).ToSQL()
	require.NoError(t, err)
	result, err := db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
}

func countLargeObjects(t *testing.T, db *sql.DB) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_largeobject_metadata").Select(goqu.COUNT("*")).ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func stopComposeService(t *testing.T, service string) {
	t.Helper()
	runComposeServiceCommand(t, "stop", service)
}

func startComposeService(t *testing.T, service string, healthURL string) {
	t.Helper()
	runComposeServiceCommand(t, "start", service)
	testenv.WaitHealthy(t, healthURL, 30*time.Second)
}

func runComposeServiceCommand(t *testing.T, action string, service string) {
	t.Helper()
	engine, baseArgs, err := testenv.FindCompose()
	require.NoError(t, err)
	args := append([]string{}, baseArgs...)
	args = append(args, "-f", composeFilePath, "-p", composeProject, action, service)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	require.NoError(t, testenv.RunComposeWithEnv(ctx, engine, composeEnv, args...))
}

type zeroReader struct{}

func (zeroReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = 0
	}
	return len(destination), nil
}
