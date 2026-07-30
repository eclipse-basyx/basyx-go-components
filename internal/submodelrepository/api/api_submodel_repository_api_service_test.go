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

package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
	"github.com/stretchr/testify/require"
)

func contextWithABACDisabled(t *testing.T) context.Context {
	t.Helper()

	cfg := &common.Config{}
	var cfgCtx context.Context
	handler := common.ConfigMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		cfgCtx = r.Context()
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotNil(t, cfgCtx)
	return cfgCtx
}

func TestResolveModelReferencePathKeysUsesEntityForParentSegment(t *testing.T) {
	t.Parallel()

	keyTypes, keyValues, err := resolveModelReferencePathKeys(
		"DemoEntity.StatementProperty1",
		"Property",
		func(path string) (string, error) {
			if path == "DemoEntity" {
				return "Entity", nil
			}
			return "", nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"Entity", "Property"}, keyTypes)
	require.Equal(t, []string{"DemoEntity", "StatementProperty1"}, keyValues)
}

func TestResolveModelReferencePathKeysUsesAnnotatedRelationshipElementForParentSegment(t *testing.T) {
	t.Parallel()

	keyTypes, keyValues, err := resolveModelReferencePathKeys(
		"DemoAnnotatedRelationshipElement.AnnotationProperty1",
		"Property",
		func(path string) (string, error) {
			if path == "DemoAnnotatedRelationshipElement" {
				return "AnnotatedRelationshipElement", nil
			}
			return "", nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"AnnotatedRelationshipElement", "Property"}, keyTypes)
	require.Equal(t, []string{"DemoAnnotatedRelationshipElement", "AnnotationProperty1"}, keyValues)
}

func TestResolveModelReferencePathKeysBuildsListIndexSegment(t *testing.T) {
	t.Parallel()

	keyTypes, keyValues, err := resolveModelReferencePathKeys(
		"test.test[0]",
		"SubmodelElementList",
		func(path string) (string, error) {
			switch path {
			case "test":
				return "SubmodelElementCollection", nil
			case "test.test":
				return "SubmodelElementCollection", nil
			default:
				return "", nil
			}
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"SubmodelElementCollection", "SubmodelElementCollection", "SubmodelElementList"}, keyTypes)
	require.Equal(t, []string{"test", "test", "0"}, keyValues)
}

func TestGetSubmodelElementByPathSubmodelRepoRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{})
	encodedSubmodelID := base64.RawStdEncoding.EncodeToString([]byte("sm-1"))

	response, err := sut.GetSubmodelElementByPathSubmodelRepo(contextWithABACDisabled(t), encodedSubmodelID, "a.b", "invalid-level", "")
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestGetSubmodelByIDPathRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{})
	encodedSubmodelID := common.EncodeString("sm-1")

	response, err := sut.GetSubmodelByIDPath(contextWithABACDisabled(t), encodedSubmodelID, "invalid-level")
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestInvokeOperationValueOnlyReturnsBadRequest(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{})
	response, err := sut.InvokeOperationValueOnly(contextWithABACDisabled(t), "", "", "", gen.OperationRequestValueOnly{}, false)
	require.NoError(t, err)
	require.Equal(t, 400, response.Code)
}

func TestInvokeOperationAsyncRequiresClientTimeoutDuration(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{})
	response, err := sut.InvokeOperationAsync(contextWithABACDisabled(t), "", "", gen.OperationRequest{})
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestOperationInvocationReadsDefinitionFromWriter(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*SubmodelRepositoryAPIAPIService, context.Context) (gen.ImplResponse, error)
	}{
		{
			name: "synchronous",
			invoke: func(service *SubmodelRepositoryAPIAPIService, ctx context.Context) (gen.ImplResponse, error) {
				return service.InvokeOperationSubmodelRepo(
					ctx,
					common.EncodeString("sm-1"),
					"operation",
					gen.OperationRequest{ClientTimeoutDuration: "PT1S"},
					false,
				)
			},
		},
		{
			name: "asynchronous",
			invoke: func(service *SubmodelRepositoryAPIAPIService, ctx context.Context) (gen.ImplResponse, error) {
				return service.InvokeOperationAsync(
					ctx,
					common.EncodeString("sm-1"),
					"operation",
					gen.OperationRequest{ClientTimeoutDuration: "PT1S"},
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer, writerMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = writer.Close() }()
			reader, readerMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = reader.Close() }()

			backend, err := persistencepostgresql.NewSubmodelDatabaseFromPools(writer, reader, nil, "off")
			require.NoError(t, err)
			writerMock.ExpectBegin()
			writerMock.ExpectQuery("SELECT").WillReturnError(errors.New("writer lookup failed"))
			writerMock.ExpectRollback()

			service := NewSubmodelRepositoryAPIAPIService(t.Context(), *backend)
			response, invokeErr := test.invoke(service, contextWithABACDisabled(t))
			require.NoError(t, invokeErr)
			require.Equal(t, http.StatusInternalServerError, response.Code)
			require.NoError(t, writerMock.ExpectationsWereMet())
			require.NoError(t, readerMock.ExpectationsWereMet())
		})
	}
}

func TestAsyncDelegationCapacityIsBounded(t *testing.T) {
	t.Parallel()

	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{})
	sut.asyncDelegationSlots = make(chan struct{}, 1)

	require.True(t, sut.tryAcquireAsyncDelegationSlot())
	require.False(t, sut.tryAcquireAsyncDelegationSlot())
	sut.releaseAsyncDelegationSlot()
	require.True(t, sut.tryAcquireAsyncDelegationSlot())
	sut.releaseAsyncDelegationSlot()
}

func TestAsyncDelegationContextStopsWithServiceLifecycle(t *testing.T) {
	t.Parallel()

	lifecycleContext, stopService := context.WithCancel(t.Context())
	sut := NewSubmodelRepositoryAPIAPIService(lifecycleContext, persistencepostgresql.SubmodelDatabase{})
	delegationContext, cancelDelegation := sut.newAsyncDelegationContext(contextWithABACDisabled(t), time.Minute)
	defer cancelDelegation()

	stopService()

	select {
	case <-delegationContext.Done():
		require.ErrorIs(t, delegationContext.Err(), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("delegation context was not canceled with the service lifecycle")
	}
}

func TestAsyncRecordExpiryStartsAtTerminalTransition(t *testing.T) {
	t.Parallel()

	retention := 5 * time.Minute
	manager := asyncjob.NewManager("SMREPO-ASYNC-TEST", retention)
	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{}, manager)
	executionDeadline := time.Now().UTC().Add(10 * time.Minute)
	handleID, err := sut.asyncJobManager.Start(t.Context(), "anonymous", asyncjob.StartOptions{
		JobKind:           "test",
		ExecutionDeadline: executionDeadline,
	})
	require.NoError(t, err)

	runningRecord, found, err := sut.asyncJobManager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, runningRecord.ExpiresAt.IsZero())
	require.Equal(t, executionDeadline, runningRecord.ExecutionDeadline)

	beforeTerminalUpdate := time.Now().UTC()
	require.NoError(t, sut.asyncJobManager.CompletePayload(t.Context(), handleID, map[string]any{"success": true}))
	terminalRecord, found, err := sut.asyncJobManager.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Completed", terminalRecord.ExecutionState)
	require.GreaterOrEqual(t, terminalRecord.ExpiresAt, beforeTerminalUpdate.Add(retention))
}

func TestPostSubmodelElementByPathSubmodelRepoMapsDeniedToForbidden(t *testing.T) {
	t.Parallel()

	response := newPostSubmodelElementByPathErrorResponse(
		common.NewErrDenied("SMREPO-ADDSMEBYPATH-CHKDUP-ABACDENIED existing submodel element is not accessible under ABAC constraints"),
	)

	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestGetOperationAsyncStatusReturnsRedirectWithLocation(t *testing.T) {
	sut := NewSubmodelRepositoryAPIAPIService(t.Context(), persistencepostgresql.SubmodelDatabase{})

	decodedSubmodelID := "sm-redirect"
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(decodedSubmodelID))
	handleID, err := sut.asyncJobManager.Start(t.Context(), "anonymous", asyncjob.StartOptions{
		JobKind: "test",
		Metadata: map[string]string{
			delegatedAsyncSubmodelIdentifierMetadataKey: decodedSubmodelID,
			delegatedAsyncIDShortPathMetadataKey:        "Ops.Add",
		},
	})
	require.NoError(t, err)
	require.NoError(t, sut.asyncJobManager.CompletePayload(t.Context(), handleID, map[string]any{"success": true}))

	response, err := sut.GetOperationAsyncStatus(contextWithABACDisabled(t), encodedSubmodelID, "Ops.Add", handleID)
	require.NoError(t, err)
	require.Equal(t, 302, response.Code)

	redirect, ok := response.Body.(openapi.Redirect)
	require.True(t, ok)
	require.True(t, strings.Contains(redirect.Location, "/operation-results/"))
}
