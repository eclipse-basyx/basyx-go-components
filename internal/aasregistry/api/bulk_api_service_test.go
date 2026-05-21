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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package aasregistryapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

type aasBulkServiceStub struct {
	createResult asyncbulk.OperationResult
	putResult    asyncbulk.OperationResult
	deleteResult asyncbulk.OperationResult
}

func (s aasBulkServiceStub) ExecuteBulkCreateAtomic(_ context.Context, _ []model.AssetAdministrationShellDescriptor) asyncbulk.OperationResult {
	return s.createResult
}

func (s aasBulkServiceStub) ExecuteBulkPutAtomic(_ context.Context, _ []model.AssetAdministrationShellDescriptor) asyncbulk.OperationResult {
	return s.putResult
}

func (s aasBulkServiceStub) ExecuteBulkDeleteAtomic(_ context.Context, _ []string) asyncbulk.OperationResult {
	return s.deleteResult
}

func TestBulkServiceResultLifecycle(t *testing.T) {
	manager := asyncbulk.NewManager("AASR-BULK-TEST", time.Minute)
	service := NewBulkService(aasBulkServiceStub{}, manager)
	handleID, err := manager.Start("anonymous")
	require.NoError(t, err)

	running := service.GetResult(context.Background(), handleID)
	require.Equal(t, http.StatusBadRequest, running.Code)

	manager.Complete(handleID, asyncbulk.OperationResult{
		Success:         true,
		ProcessedCount:  1,
		SuccessfulCount: 1,
		FailedCount:     0,
	})

	found := service.GetStatus(context.Background(), handleID)
	require.Equal(t, http.StatusFound, found.Code)

	success := service.GetResult(context.Background(), handleID)
	require.Equal(t, http.StatusNoContent, success.Code)

	notFound := service.GetResult(context.Background(), handleID)
	require.Equal(t, http.StatusNotFound, notFound.Code)
}

func TestBulkServiceStatusIsOwnerScoped(t *testing.T) {
	manager := asyncbulk.NewManager("AASR-BULK-TEST", time.Minute)
	service := NewBulkService(aasBulkServiceStub{}, manager)

	ownerCtx := withClaims(context.Background(), auth.Claims{"sub": "owner-a", "iss": "issuer-a"})
	otherCtx := withClaims(context.Background(), auth.Claims{"sub": "owner-b", "iss": "issuer-a"})

	start := service.StartCreate(ownerCtx, []model.AssetAdministrationShellDescriptor{{Id: "id-1"}})
	require.Equal(t, http.StatusAccepted, start.Code)
	handleID := extractAASHandleID(t, start)

	require.Equal(t, http.StatusNotFound, service.GetStatus(otherCtx, handleID).Code)
}

func TestBulkServiceCreateFailureResult(t *testing.T) {
	manager := asyncbulk.NewManager("AASR-BULK-TEST", time.Minute)
	service := NewBulkService(aasBulkServiceStub{
		createResult: asyncbulk.OperationResult{
			Success:         false,
			ProcessedCount:  2,
			SuccessfulCount: 0,
			FailedCount:     2,
			Failures: []asyncbulk.ItemFailure{
				{Index: 1, Identifier: "bad-id", StatusCode: http.StatusConflict, Message: "conflict"},
			},
		},
	}, manager)

	start := service.StartCreate(context.Background(), []model.AssetAdministrationShellDescriptor{{Id: "id-1"}, {Id: "bad-id"}})
	require.Equal(t, http.StatusAccepted, start.Code)
	handleID := extractAASHandleID(t, start)
	awaitAASResultAvailability(t, service, handleID)

	result := service.GetResult(context.Background(), handleID)
	require.Equal(t, http.StatusBadRequest, result.Code)

	body := result.Body.(map[string]any)
	require.EqualValues(t, 2, body["processedCount"])
	require.EqualValues(t, 0, body["successfulCount"])
	require.EqualValues(t, 2, body["failedCount"])
	messages, ok := body["messages"].([]model.Message)
	require.True(t, ok)
	require.Len(t, messages, 1)
	require.Equal(t, "Error", messages[0].MessageType)
}

func awaitAASResultAvailability(t *testing.T, service *BulkService, handleID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := service.GetStatus(context.Background(), handleID)
		if status.Code == http.StatusFound {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("AASR-BULK-TEST-TIMEOUT %s", handleID)
}

func extractAASHandleID(t *testing.T, response model.ImplResponse) string {
	t.Helper()

	redirect, ok := response.Body.(model.Redirect)
	require.True(t, ok)
	handleID := redirect.Location[strings.LastIndex(redirect.Location, "/")+1:]
	require.NotEmpty(t, handleID)
	return handleID
}

func withClaims(ctx context.Context, claims auth.Claims) context.Context {
	if ctx == nil {
		ctx = context.TODO()
	}

	return context.WithValue(ctx, auth.ClaimsKey, claims)
}
