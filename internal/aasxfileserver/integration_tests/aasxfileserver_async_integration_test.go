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
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
	"github.com/stretchr/testify/require"
)

const (
	ssp001Profile  = "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-001"
	ssp002Profile  = "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-002"
	validAASXPath  = "../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx"
	asyncDeadline  = 45 * time.Second
	acceptDeadline = 5 * time.Second
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

type asyncRequestResult struct {
	response asyncResponse
	err      error
}

func TestAsyncUploadCompletesAfterRequestCleanupAndAcrossReplicas(t *testing.T) {
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	db := openIntegrationDatabase(t)
	largeObjectsBefore := largeObjectOIDs(t, db)
	releasePersistence := blockPackagePersistence(t, db)

	requestResult := startAsyncFileRequest(t, baseURL, validAASXPath, nil, "")
	accepted := awaitAcceptedRequest(t, requestResult, releasePersistence)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	assertRunningStatus(t, replicaBURL, handle, "")
	stagedOID := addedLargeObjectOID(t, largeObjectsBefore, largeObjectOIDs(t, db))
	releasePersistence()

	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Completed", result.ExecutionState)
	require.True(t, result.Success)

	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	require.Equal(t, stagedOID, packageLargeObjectOID(t, db, created.PackageId), "package persistence copied the staged upload instead of promoting it")
	t.Cleanup(func() {
		deletePackage(t, replicaBURL, created.PackageId, "")
	})

	download := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages/"+url.PathEscape(created.PackageId), nil, "")
	require.Equal(t, http.StatusOK, download.status)
	expected, err := os.ReadFile(filepath.Clean(validAASXPath))
	require.NoError(t, err)
	require.Equal(t, expected, download.body)
	require.Equal(t, filepath.Base(validAASXPath), created.FileName)
}

func TestAsyncUploadPreservesAASIDsAndSupportsFiltering(t *testing.T) {
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)

	aasIDOne := "urn:example:aas:async-one"
	aasIDTwo := "https://example.com/aas/async-two"
	accepted := postAsyncFileWithAASIDs(t, baseURL, validAASXPath, []string{
		" " + aasIDOne + ", " + aasIDTwo + " ",
		" , " + aasIDOne + ", , ",
	})
	handle := assertAcceptedOperation(t, accepted, baseURL)
	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Completed", result.ExecutionState)
	require.True(t, result.Success)

	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	t.Cleanup(func() { deletePackage(t, replicaBURL, created.PackageId, "") })
	require.Equal(t, []string{common.EncodeString(aasIDOne), common.EncodeString(aasIDTwo)}, created.AasIds)

	for _, aasID := range []string{aasIDOne, aasIDTwo} {
		filtered := doAsyncRequest(t, http.MethodGet, replicaBURL+"/packages?aasId="+url.QueryEscape(common.EncodeString(aasID)), nil, "")
		require.Equal(t, http.StatusOK, filtered.status)
		var result openapi.GetPackageDescriptionsResult
		require.NoError(t, json.Unmarshal(filtered.body, &result))
		require.Contains(t, collectPackageIDs(result.Result), created.PackageId)
	}
}

func TestAsyncHandleRemainsOwnerScopedAcrossReplicas(t *testing.T) {
	ownerAToken := accessToken(t, "usera")
	ownerBToken := accessToken(t, "userb")
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	db := openIntegrationDatabase(t)
	asyncJobsBefore := countAsyncJobs(t, db)
	largeObjectsBefore := countLargeObjects(t, db)

	anonymousPost := postAsyncFile(t, secureReplicaBURL, validAASXPath)
	require.Equalf(t, http.StatusUnauthorized, anonymousPost.status, "anonymous POST response: %s", anonymousPost.body)
	require.Equal(t, asyncJobsBefore, countAsyncJobs(t, db), "anonymous upload created an async handle")
	require.Equal(t, largeObjectsBefore, countLargeObjects(t, db), "anonymous upload created a staged large object")
	packagesAfterAnonymousPost, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	require.ElementsMatch(t, collectPackageIDs(packagesBefore.Result), collectPackageIDs(packagesAfterAnonymousPost.Result))

	accepted := postAsyncFileWithAuthorization(t, secureBaseURL, validAASXPath, ownerAToken)
	handle := assertAcceptedOperation(t, accepted, secureBaseURL)
	for _, endpoint := range []string{"status", "result"} {
		knownURL := secureReplicaBURL + "/packages-async/" + endpoint + "/" + url.PathEscape(handle)
		anonymous := doAsyncRequest(t, http.MethodGet, knownURL, nil, "")
		require.Equalf(t, http.StatusUnauthorized, anonymous.status, "anonymous %s response: %s", endpoint, anonymous.body)

		foreign := doAuthorizedAsyncRequest(t, http.MethodGet, knownURL, nil, "", ownerBToken)
		unknown := doAuthorizedAsyncRequest(t, http.MethodGet, secureReplicaBURL+"/packages-async/"+endpoint+"/unknown-owner-handle", nil, "", ownerBToken)
		require.Equalf(t, http.StatusNotFound, foreign.status, "owner B %s response: %s", endpoint, foreign.body)
		require.Equalf(t, http.StatusNotFound, unknown.status, "unknown %s response: %s", endpoint, unknown.body)
		require.Equal(t, normalizedErrorPayload(t, unknown.body), normalizedErrorPayload(t, foreign.body), "foreign handles must not be distinguishable from unknown handles")
	}

	result := awaitAsyncResultWithAuthorization(t, secureReplicaBURL, handle, ownerAToken)
	require.Equal(t, "Completed", result.ExecutionState)
	require.True(t, result.Success)
	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	t.Cleanup(func() { deletePackage(t, replicaBURL, created.PackageId, "") })
}

func TestAsyncStatusAndResultContracts(t *testing.T) {
	t.Run("unknown handle", func(t *testing.T) {
		for _, endpoint := range []string{"status", "result"} {
			response := doAsyncRequest(t, http.MethodGet, baseURL+"/packages-async/"+endpoint+"/unknown-handle", nil, "")
			require.Equalf(t, http.StatusNotFound, response.status, "%s response: %s", endpoint, response.body)
		}
	})

	t.Run("terminal status redirects without client auto follow", func(t *testing.T) {
		packagesBefore, statusCode := listPackagesFrom(t, baseURL)
		require.Equal(t, http.StatusOK, statusCode)
		accepted := postAsyncFile(t, baseURL, validAASXPath)
		handle := assertAcceptedOperation(t, accepted, baseURL)
		statusResponse := awaitTerminalStatus(t, replicaBURL, handle)
		assertHandleLocation(t, statusResponse.header.Get("Location"), replicaBURL, "/packages-async/result/", handle)

		resultResponse := doAsyncRequest(t, http.MethodGet, resolveLocation(t, replicaBURL, statusResponse.header.Get("Location")), nil, "")
		require.Equal(t, http.StatusOK, resultResponse.status)
		require.Equal(t, "application/json", mediaType(t, resultResponse.header.Get("Content-Type")))
		result := decodeBaseOperationResult(t, resultResponse.body)
		require.Contains(t, []string{"Completed", "Failed", "Canceled", "Timeout"}, result.ExecutionState)
		if result.ExecutionState == "Completed" {
			packagesAfter, packageStatus := listPackagesFrom(t, replicaBURL)
			require.Equal(t, http.StatusOK, packageStatus)
			created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
			t.Cleanup(func() { deletePackage(t, replicaBURL, created.PackageId, "") })
		}
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
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	accepted := postAsyncFile(t, contextBaseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, contextBaseURL)
	location := accepted.header.Get("Location")
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	require.Equal(t, "/external/aasx/packages-async/status/"+url.PathEscape(handle), parsed.Path)
	require.NotContains(t, location, "aasx_fileserver_context_it")
	result := awaitAsyncResult(t, contextBaseURL, handle)
	require.Equal(t, "Completed", result.ExecutionState)
	packagesAfter, status := listPackagesFrom(t, contextBaseURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	t.Cleanup(func() { deletePackage(t, contextBaseURL, created.PackageId, "") })
}

func TestAsyncTerminalRetentionStartsAtCompletionAndExpiresThroughCleanup(t *testing.T) {
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	accepted := postAsyncFile(t, baseURL, validAASXPath)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	_ = awaitAsyncResult(t, replicaBURL, handle)
	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	t.Cleanup(func() { deletePackage(t, replicaBURL, created.PackageId, "") })
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
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	db := openIntegrationDatabase(t)
	releasePersistence := blockPackagePersistence(t, db)
	requestResult := startAsyncFileRequest(t, baseURL, validAASXPath, nil, "")
	accepted := awaitAcceptedRequest(t, requestResult, releasePersistence)
	handle := assertAcceptedOperation(t, accepted, baseURL)

	query, args, err := goqu.Dialect("postgres").Update("async_job").
		Set(goqu.Record{"expires_at": time.Now().UTC().Add(-time.Second)}).
		Where(goqu.C("handle_id").Eq(handle)).ToSQL()
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

	releasePersistence()
	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Completed", result.ExecutionState)
	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	created := packageDifference(t, packagesBefore.Result, packagesAfter.Result)
	t.Cleanup(func() { deletePackage(t, replicaBURL, created.PackageId, "") })
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
	largeObjectsBefore := countLargeObjects(t, db)
	packagesBefore, status := listPackagesFrom(t, baseURL)
	require.Equal(t, http.StatusOK, status)
	releasePersistence := blockPackagePersistence(t, db)
	requestResult := startAsyncFileRequest(t, baseURL, validAASXPath, nil, "")
	accepted := awaitAcceptedRequest(t, requestResult, releasePersistence)
	handle := assertAcceptedOperation(t, accepted, baseURL)
	assertRunningStatus(t, replicaBURL, handle, "")
	require.Equal(t, largeObjectsBefore+1, countLargeObjects(t, db), "running upload was not durably staged")

	stopComposeService(t, "aasx_fileserver_it")
	t.Cleanup(func() { startComposeService(t, "aasx_fileserver_it", baseURL+"/health") })
	releasePersistence()
	expireHandleLease(t, db, handle)

	result := awaitAsyncResult(t, replicaBURL, handle)
	require.Equal(t, "Failed", result.ExecutionState)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Messages)
	packagesAfter, status := listPackagesFrom(t, replicaBURL)
	require.Equal(t, http.StatusOK, status)
	require.ElementsMatch(t, collectPackageIDs(packagesBefore.Result), collectPackageIDs(packagesAfter.Result))
	require.Eventually(t, func() bool { return countLargeObjects(t, db) == largeObjectsBefore }, 5*time.Second, 50*time.Millisecond)
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
	return awaitAsyncResultWithAuthorization(t, pollingBaseURL, handle, "")
}

func awaitAsyncResultWithAuthorization(t *testing.T, pollingBaseURL string, handle string, token string) baseOperationResult {
	t.Helper()
	terminal := awaitTerminalStatusWithAuthorization(t, pollingBaseURL, handle, token)
	resultURL := resolveLocation(t, pollingBaseURL, terminal.header.Get("Location"))
	resultResponse := doAuthorizedAsyncRequest(t, http.MethodGet, resultURL, nil, "", token)
	require.Equalf(t, http.StatusOK, resultResponse.status, "result response: %s", resultResponse.body)
	require.Equal(t, "application/json", mediaType(t, resultResponse.header.Get("Content-Type")))
	return decodeBaseOperationResult(t, resultResponse.body)
}

func awaitTerminalStatus(t *testing.T, pollingBaseURL string, handle string) asyncResponse {
	return awaitTerminalStatusWithAuthorization(t, pollingBaseURL, handle, "")
}

func awaitTerminalStatusWithAuthorization(t *testing.T, pollingBaseURL string, handle string, token string) asyncResponse {
	t.Helper()
	deadline := time.Now().Add(asyncDeadline)
	for time.Now().Before(deadline) {
		response := doAuthorizedAsyncRequest(t, http.MethodGet, pollingBaseURL+"/packages-async/status/"+url.PathEscape(handle), nil, "", token)
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
	return postAsyncFileWithOptions(t, serviceURL, path, nil, "")
}

func postAsyncFileWithAASIDs(t *testing.T, serviceURL string, path string, aasIDFields []string) asyncResponse {
	return postAsyncFileWithOptions(t, serviceURL, path, aasIDFields, "")
}

func postAsyncFileWithAuthorization(t *testing.T, serviceURL string, path string, token string) asyncResponse {
	return postAsyncFileWithOptions(t, serviceURL, path, nil, token)
}

func postAsyncFileWithOptions(t *testing.T, serviceURL string, path string, aasIDFields []string, token string) asyncResponse {
	t.Helper()
	file, err := os.Open(filepath.Clean(path))
	require.NoError(t, err)
	response := postAsyncReaderWithOptions(t, serviceURL, filepath.Base(path), file, aasIDFields, token)
	require.NoError(t, file.Close())
	return response
}

func postAsyncReader(t *testing.T, serviceURL string, filename string, content io.Reader) asyncResponse {
	return postAsyncReaderWithOptions(t, serviceURL, filename, content, nil, "")
}

func postAsyncReaderWithOptions(t *testing.T, serviceURL string, filename string, content io.Reader, aasIDFields []string, token string) asyncResponse {
	t.Helper()
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	writeDone := make(chan error, 1)
	go func() {
		part, err := multipartWriter.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, content)
		}
		for _, aasIDField := range aasIDFields {
			if err == nil {
				err = multipartWriter.WriteField("aasIds", aasIDField)
			}
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
		writeDone <- err
	}()
	response := doAuthorizedAsyncRequest(t, http.MethodPost, serviceURL+"/packages-async", reader, multipartWriter.FormDataContentType(), token)
	writeErr := <-writeDone
	if response.status == http.StatusAccepted {
		require.NoError(t, writeErr)
	}
	return response
}

func startAsyncFileRequest(t *testing.T, serviceURL string, path string, aasIDFields []string, token string) <-chan asyncRequestResult {
	t.Helper()
	payload, contentType := multipartFilePayload(t, path, aasIDFields)
	result := make(chan asyncRequestResult, 1)
	go func() {
		response, err := executeAsyncAcceptanceRequest(
			http.MethodPost,
			serviceURL+"/packages-async",
			bytes.NewReader(payload),
			contentType,
			token,
		)
		result <- asyncRequestResult{response: response, err: err}
	}()
	return result
}

func multipartFilePayload(t *testing.T, path string, aasIDFields []string) ([]byte, string) {
	t.Helper()
	file, err := os.Open(filepath.Clean(path))
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	require.NoError(t, err)
	_, err = io.Copy(part, file)
	require.NoError(t, err)
	for _, aasIDField := range aasIDFields {
		require.NoError(t, writer.WriteField("aasIds", aasIDField))
	}
	require.NoError(t, writer.Close())
	return payload.Bytes(), writer.FormDataContentType()
}

func awaitAcceptedRequest(t *testing.T, result <-chan asyncRequestResult, unblock func()) asyncResponse {
	t.Helper()
	timer := time.NewTimer(acceptDeadline)
	defer timer.Stop()
	select {
	case requestResult := <-result:
		require.NoError(t, requestResult.err)
		return requestResult.response
	case <-timer.C:
		unblock()
		select {
		case delayedResult := <-result:
			require.NoError(t, delayedResult.err)
			t.Fatalf("POST /packages-async did not return before package processing was unblocked; delayed status: %d", delayedResult.response.status)
		case <-time.After(acceptDeadline):
			t.Fatal("POST /packages-async remained blocked after package processing was unblocked")
		}
	}
	return asyncResponse{}
}

func doAsyncRequest(t *testing.T, method string, endpoint string, body io.Reader, contentType string) asyncResponse {
	return doAuthorizedAsyncRequest(t, method, endpoint, body, contentType, "")
}

func doAuthorizedAsyncRequest(t *testing.T, method string, endpoint string, body io.Reader, contentType string, token string) asyncResponse {
	t.Helper()
	response, err := executeAsyncRequestWithAuthorization(method, endpoint, body, contentType, token)
	require.NoError(t, err)
	return response
}

func executeAsyncRequestWithAuthorization(method string, endpoint string, body io.Reader, contentType string, token string) (asyncResponse, error) {
	return executeAsyncHTTPRequest(method, endpoint, body, contentType, token, false)
}

func executeAsyncAcceptanceRequest(method string, endpoint string, body io.Reader, contentType string, token string) (asyncResponse, error) {
	return executeAsyncHTTPRequest(method, endpoint, body, contentType, token, true)
}

func executeAsyncHTTPRequest(method string, endpoint string, body io.Reader, contentType string, token string, expectContinue bool) (asyncResponse, error) {
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return asyncResponse{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if expectContinue {
		req.Header.Set("Expect", "100-continue")
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
	if response.status != http.StatusOK {
		return result, response.status
	}
	require.NoError(t, json.Unmarshal(response.body, &result))
	return result, response.status
}

func deletePackage(t *testing.T, serviceURL string, packageID string, token string) {
	t.Helper()
	response := doAuthorizedAsyncRequest(t, http.MethodDelete, serviceURL+"/packages/"+url.PathEscape(packageID), nil, "", token)
	require.Equalf(t, http.StatusNoContent, response.status, "delete response: %s", response.body)
}

func assertRunningStatus(t *testing.T, serviceURL string, handle string, token string) {
	t.Helper()
	response := doAuthorizedAsyncRequest(t, http.MethodGet, serviceURL+"/packages-async/status/"+url.PathEscape(handle), nil, "", token)
	require.Equalf(t, http.StatusOK, response.status, "running status response: %s", response.body)
	result := decodeBaseOperationResult(t, response.body)
	require.Equal(t, "Running", result.ExecutionState)
	require.True(t, result.Success)
}

func accessToken(t *testing.T, username string) string {
	t.Helper()
	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {"basyx-ui"},
		"username":   {username},
		"password":   {"pwd"},
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, securityKeycloakURL, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	request.Host = "keycloak:8080"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, response.StatusCode, "token response: %s", body)
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(body, &tokenResponse))
	require.NotEmpty(t, tokenResponse.AccessToken)
	return tokenResponse.AccessToken
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

func normalizedErrorPayload(t *testing.T, body []byte) any {
	t.Helper()
	var payload any
	require.NoError(t, json.Unmarshal(body, &payload))
	removeVolatileErrorFields(payload)
	return payload
}

func removeVolatileErrorFields(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "correlationId")
		delete(typed, "timestamp")
		for _, child := range typed {
			removeVolatileErrorFields(child)
		}
	case []any:
		for _, child := range typed {
			removeVolatileErrorFields(child)
		}
	}
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

func expireHandleLease(t *testing.T, db *sql.DB, handle string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Update("async_job").
		Set(goqu.Record{"lease_expires_at": time.Now().UTC().Add(-time.Second)}).
		Where(goqu.C("handle_id").Eq(handle)).ToSQL()
	require.NoError(t, err)
	result, err := db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
}

func blockPackagePersistence(t *testing.T, db *sql.DB) func() {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), "LOCK TABLE aasx_package IN ACCESS EXCLUSIVE MODE")
	require.NoError(t, err)

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			_ = tx.Rollback()
		})
	}
	t.Cleanup(release)
	return release
}

func countLargeObjects(t *testing.T, db *sql.DB) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_largeobject_metadata").Select(goqu.COUNT("*")).ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func countAsyncJobs(t *testing.T, db *sql.DB) int {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("async_job").Select(goqu.COUNT("*")).ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&count))
	return count
}

func largeObjectOIDs(t *testing.T, db *sql.DB) []int64 {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_largeobject_metadata").
		Select(goqu.C("oid")).Order(goqu.C("oid").Asc()).ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	oids := make([]int64, 0)
	for rows.Next() {
		var oid int64
		require.NoError(t, rows.Scan(&oid))
		oids = append(oids, oid)
	}
	require.NoError(t, rows.Err())
	return oids
}

func addedLargeObjectOID(t *testing.T, before []int64, after []int64) int64 {
	t.Helper()
	known := make(map[int64]struct{}, len(before))
	for _, oid := range before {
		known[oid] = struct{}{}
	}
	added := make([]int64, 0, 1)
	for _, oid := range after {
		if _, found := known[oid]; !found {
			added = append(added, oid)
		}
	}
	require.Len(t, added, 1, "accepted upload must create exactly one durable staged large object")
	return added[0]
}

func packageLargeObjectOID(t *testing.T, db *sql.DB, encodedPackageID string) int64 {
	t.Helper()
	packageID, err := common.DecodeString(encodedPackageID)
	require.NoError(t, err)
	query, args, err := goqu.Dialect("postgres").From("aasx_package").
		Select(goqu.C("file_oid")).Where(goqu.C("package_id").Eq(packageID)).ToSQL()
	require.NoError(t, err)
	var oid int64
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&oid))
	return oid
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
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
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
