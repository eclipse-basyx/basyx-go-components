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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
	"github.com/stretchr/testify/require"
)

var errAsyncPackageCreationIntercepted = errors.New("async package creation intercepted")

type asyncPackagePoster interface {
	PostAsyncAASXPackage(context.Context, openapi.StagedUpload, []string, string) (openapi.ImplResponse, error)
}

type workerReadTrackingUpload struct {
	io.ReadSeeker
	size             int64
	closed           chan struct{}
	closeOnce        sync.Once
	readByWorker     atomic.Bool
	promotedByWorker atomic.Bool
}

func (upload *workerReadTrackingUpload) Read(destination []byte) (int, error) {
	upload.readByWorker.Store(true)
	return upload.ReadSeeker.Read(destination)
}

func (upload *workerReadTrackingUpload) Size() int64 {
	return upload.size
}

func (upload *workerReadTrackingUpload) Promote(context.Context, func(context.Context, *sql.Tx, int64, int64) error) error {
	upload.promotedByWorker.Store(true)
	return errAsyncPackageCreationIntercepted
}

func (upload *workerReadTrackingUpload) Close() error {
	upload.closeOnce.Do(func() { close(upload.closed) })
	return nil
}

type packageCreatorSpy struct {
	called           chan struct{}
	callOnce         sync.Once
	receivedUpload   common.StagedUpload
	receivedFileName string
}

type panickingPackageCreator struct{}

type failingPackageCreator struct {
	err error
}

func (panickingPackageCreator) CreatePackage(
	context.Context,
	string,
	common.StagedUpload,
	[]string,
	string,
) (*persistence.PackageRecord, error) {
	panic("sensitive parser panic")
}

func (creator failingPackageCreator) CreatePackage(
	context.Context,
	string,
	common.StagedUpload,
	[]string,
	string,
) (*persistence.PackageRecord, error) {
	return nil, creator.err
}

func (creator *packageCreatorSpy) CreatePackage(
	_ context.Context,
	_ string,
	upload common.StagedUpload,
	_ []string,
	fileName string,
) (*persistence.PackageRecord, error) {
	creator.receivedUpload = upload
	creator.receivedFileName = fileName
	creator.callOnce.Do(func() { close(creator.called) })
	return nil, errAsyncPackageCreationIntercepted
}

func TestAsyncPackageWorkerHandsStagedUploadToPersistenceWithoutPreReading(t *testing.T) {
	filePath := filepath.Clean("../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx")
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	fileInfo, err := file.Stat()
	require.NoError(t, err)

	creator := &packageCreatorSpy{called: make(chan struct{})}
	service := NewAASXFileServerAPIAPIService(nil)
	service.packageCreator = creator
	manager := asyncjob.NewManager("AASXFS-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "test-owner", asyncjob.StartOptions{JobKind: asyncPackageJobKind})
	require.NoError(t, err)

	upload := &workerReadTrackingUpload{
		ReadSeeker: file,
		size:       fileInfo.Size(),
		closed:     make(chan struct{}),
	}
	expectedFileName := filepath.Base(filePath)
	go service.processAsyncPackage(t.Context(), manager, handleID, upload, nil, expectedFileName, func() {}, func() {})

	select {
	case <-creator.called:
	case <-time.After(5 * time.Second):
		t.Fatal("async worker did not hand the staged upload to persistence")
	}
	require.Same(t, upload, creator.receivedUpload, "async worker did not pass the original staged upload to package persistence")
	require.Equal(t, expectedFileName, creator.receivedFileName, "async worker did not preserve the multipart source filename")
	require.False(t, upload.readByWorker.Load(), "async worker read the staged package before persistence received it")
	require.False(t, upload.promotedByWorker.Load(), "async worker promoted the staged package instead of passing it to persistence")
	select {
	case <-upload.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("async worker did not release the staged upload after persistence failed")
	}
	require.False(t, upload.readByWorker.Load(), "async worker read the staged package after persistence returned")
}

func TestAsyncPackageWorkerRecoversPanicAndPersistsGenericFailure(t *testing.T) {
	service := NewAASXFileServerAPIAPIService(nil)
	service.packageCreator = panickingPackageCreator{}
	manager := asyncjob.NewManager("AASXFS-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "test-owner", asyncjob.StartOptions{JobKind: asyncPackageJobKind})
	require.NoError(t, err)
	upload := newWorkerReadTrackingUpload()
	cancelled := false
	released := false

	service.processAsyncPackage(
		t.Context(),
		manager,
		handleID,
		upload,
		nil,
		"package.aasx",
		func() { cancelled = true },
		func() { released = true },
	)

	record, found, err := manager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Failed", record.ExecutionState)
	require.Equal(t, http.StatusInternalServerError, record.ErrorStatus)
	result, ok := record.ErrorBody.(openapi.BaseOperationResult)
	require.True(t, ok)
	require.False(t, result.Success)
	require.Equal(t, openapi.EXECUTIONSTATE_FAILED, result.ExecutionState)
	require.Len(t, result.Messages, 1)
	require.Equal(t, asyncPackageProcessingFailureMessage, result.Messages[0]["text"])
	require.NotContains(t, result.Messages[0]["text"], "sensitive parser panic")
	require.True(t, cancelled)
	require.True(t, released)
	select {
	case <-upload.closed:
	default:
		t.Fatal("async worker did not close the staged upload after recovering a panic")
	}
}

func TestAsyncPackageWorkerSanitizesClientVisibleFailures(t *testing.T) {
	tests := []struct {
		name            string
		creationError   error
		expectedMessage string
	}{
		{
			name:            "internal error",
			creationError:   common.NewInternalServerError("sensitive database detail"),
			expectedMessage: asyncPackageProcessingFailureMessage,
		},
		{
			name:            "invalid package",
			creationError:   common.NewErrBadRequest("AASXFS-TEST-INVALID invalid AASX package"),
			expectedMessage: "400 Bad Request: AASXFS-TEST-INVALID invalid AASX package",
		},
		{
			name:            "package limit exceeded",
			creationError:   common.NewErrPayloadTooLarge("AASXFS-TEST-LIMIT expanded package exceeds configured limit"),
			expectedMessage: "413 Payload Too Large: AASXFS-TEST-LIMIT expanded package exceeds configured limit",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runFailingAsyncPackageWorker(t, test.creationError)
			require.Len(t, result.Messages, 1)
			require.Equal(t, test.expectedMessage, result.Messages[0]["text"])
			require.NotContains(t, result.Messages[0]["text"], "sensitive database detail")
		})
	}
}

func TestGeneratePackageIDIsUniqueUnderConcurrency(t *testing.T) {
	const generatedIDCount = 1_000
	type generationResult struct {
		packageID string
		err       error
	}
	generatedIDs := make(chan generationResult, generatedIDCount)
	for range generatedIDCount {
		go func() {
			packageID, err := generatePackageID()
			generatedIDs <- generationResult{packageID: packageID, err: err}
		}()
	}

	uniqueIDs := make(map[string]struct{}, generatedIDCount)
	for range generatedIDCount {
		result := <-generatedIDs
		require.NoError(t, result.err)
		packageID := result.packageID
		require.True(t, strings.HasPrefix(packageID, "pkg-"))
		_, duplicate := uniqueIDs[packageID]
		require.Falsef(t, duplicate, "duplicate generated package ID: %s", packageID)
		uniqueIDs[packageID] = struct{}{}
	}
	require.Len(t, uniqueIDs, generatedIDCount)
}

func TestPostAsyncAASXPackageFailsWhenPersistenceIsNotConfigured(t *testing.T) {
	service := NewAASXFileServerAPIAPIService(nil)
	asyncService, implemented := any(service).(asyncPackagePoster)
	require.True(t, implemented, "AASX service must implement the generated async package API")

	upload := newWorkerReadTrackingUpload()
	defer func() { require.NoError(t, upload.Close()) }()
	response, err := asyncService.PostAsyncAASXPackage(t.Context(), upload, nil, "package.aasx")
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, response.Code)
}

func TestPostAsyncAASXPackageRejectsExhaustedExecutionCapacity(t *testing.T) {
	manager := asyncjob.NewManager("AASXFS-TEST", time.Minute)
	releases := reserveAllExecutionSlots(manager)
	defer releaseExecutionSlots(releases)
	service := NewAASXFileServerAPIAPIService(nil, WithAsyncPackageUploads(manager, &persistence.AsyncUploadStore{}))

	upload := newWorkerReadTrackingUpload()
	defer func() { require.NoError(t, upload.Close()) }()
	response, err := service.PostAsyncAASXPackage(t.Context(), upload, nil, "package.aasx")
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, response.Code)
}

func TestPostAsyncAASXPackageReleasesCapacityAfterAcceptanceFailure(t *testing.T) {
	manager, err := asyncjob.NewManagerWithExecutionCapacity("AASXFS-TEST", time.Minute, 1)
	require.NoError(t, err)
	service := NewAASXFileServerAPIAPIService(nil, WithAsyncPackageUploads(manager, &persistence.AsyncUploadStore{}))
	executionSlot, acquired := manager.TryAcquireExecutionSlotLease()
	require.True(t, acquired)
	requestContext := asyncjob.WithExecutionSlotLease(t.Context(), executionSlot)

	upload := newWorkerReadTrackingUpload()
	defer func() { require.NoError(t, upload.Close()) }()
	response, err := service.PostAsyncAASXPackage(requestContext, upload, nil, "package.aasx")
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	responseBody, err := json.Marshal(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(responseBody), asyncPackageAcceptanceFailureMessage)
	require.NotContains(t, string(responseBody), errAsyncPackageCreationIntercepted.Error())
	executionSlot.ReleaseIfUnclaimed()

	release, acquired := manager.TryAcquireExecutionSlot()
	require.True(t, acquired, "acceptance failure leaked its execution slot")
	release()
}

func TestAsyncStatusSanitizesPersistenceReadFailure(t *testing.T) {
	const sensitiveDetail = "sensitive database connection detail"
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	mock.ExpectExec(`UPDATE .*async_job`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM .*async_job`).WillReturnResult(sqlmock.NewResult(0, 0))

	manager, err := asyncjob.NewPostgresManager(t.Context(), database, "AASXFS-TEST", time.Minute)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT .*async_job`).WillReturnError(errors.New(sensitiveDetail))

	service := NewAASXFileServerAPIAPIService(nil)
	service.asyncJobs = manager
	response, err := service.GetAasxAsyncStatus(t.Context(), "test-handle")
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	responseBody, err := json.Marshal(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(responseBody), asyncPackageStateReadFailureMessage)
	require.NotContains(t, string(responseBody), sensitiveDetail)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAasxAsyncResultPreservesTypedFailure(t *testing.T) {
	expected := failedOperationResult("package validation failed")
	tests := []struct {
		name    string
		payload any
	}{
		{name: "value", payload: expected},
		{name: "pointer", payload: &expected},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := asyncjob.NewManager("AASXFS-TEST", time.Minute)
			handleID, err := manager.Start(t.Context(), "anonymous", asyncjob.StartOptions{JobKind: asyncPackageJobKind})
			require.NoError(t, err)
			require.NoError(t, manager.Fail(t.Context(), handleID, http.StatusInternalServerError, test.payload))
			service := NewAASXFileServerAPIAPIService(nil, WithAsyncPackageUploads(manager, &persistence.AsyncUploadStore{}))

			response, err := service.GetAasxAsyncResult(t.Context(), handleID)

			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, expected, response.Body)
		})
	}
}

func newWorkerReadTrackingUpload() *workerReadTrackingUpload {
	return &workerReadTrackingUpload{
		ReadSeeker: strings.NewReader(""),
		closed:     make(chan struct{}),
	}
}

func runFailingAsyncPackageWorker(t *testing.T, creationError error) openapi.BaseOperationResult {
	t.Helper()
	service := NewAASXFileServerAPIAPIService(nil)
	service.packageCreator = failingPackageCreator{err: creationError}
	manager := asyncjob.NewManager("AASXFS-TEST", time.Minute)
	handleID, err := manager.Start(t.Context(), "test-owner", asyncjob.StartOptions{JobKind: asyncPackageJobKind})
	require.NoError(t, err)

	service.processAsyncPackage(
		t.Context(),
		manager,
		handleID,
		newWorkerReadTrackingUpload(),
		nil,
		"package.aasx",
		func() {},
		func() {},
	)

	record, found, err := manager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Failed", record.ExecutionState)
	result, ok := record.ErrorBody.(openapi.BaseOperationResult)
	require.True(t, ok)
	return result
}

func reserveAllExecutionSlots(manager *asyncjob.Manager) []func() {
	releases := make([]func(), 0)
	for {
		release, acquired := manager.TryAcquireExecutionSlot()
		if !acquired {
			return releases
		}
		releases = append(releases, release)
	}
}

func releaseExecutionSlots(releases []func()) {
	for _, release := range releases {
		release()
	}
}
