package smregistryapi

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

type smBulkServiceStub struct {
	createResult asyncbulk.OperationResult
	putResult    asyncbulk.OperationResult
	deleteResult asyncbulk.OperationResult
}

func (s smBulkServiceStub) ExecuteBulkCreateAtomic(_ context.Context, _ []model.SubmodelDescriptor) asyncbulk.OperationResult {
	return s.createResult
}

func (s smBulkServiceStub) ExecuteBulkPutAtomic(_ context.Context, _ []model.SubmodelDescriptor) asyncbulk.OperationResult {
	return s.putResult
}

func (s smBulkServiceStub) ExecuteBulkDeleteAtomic(_ context.Context, _ []string) asyncbulk.OperationResult {
	return s.deleteResult
}

func TestBulkServiceResultLifecycle(t *testing.T) {
	manager := asyncbulk.NewManager("SMR-BULK-TEST", time.Minute)
	service := NewBulkService(smBulkServiceStub{}, manager)
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

func TestBulkServiceResultIsOwnerScoped(t *testing.T) {
	manager := asyncbulk.NewManager("SMR-BULK-TEST", time.Minute)
	service := NewBulkService(smBulkServiceStub{}, manager)

	ownerCtx := auth.WithClaims(context.Background(), auth.Claims{"sub": "owner-a", "iss": "issuer-a"})
	otherCtx := auth.WithClaims(context.Background(), auth.Claims{"sub": "owner-b", "iss": "issuer-a"})

	start := service.StartDelete(ownerCtx, []string{"urn:ok"})
	require.Equal(t, http.StatusAccepted, start.Code)

	location := start.Body.(model.Redirect).Location
	handleID := location[strings.LastIndex(location, "/")+1:]

	require.Equal(t, http.StatusNotFound, service.GetResult(otherCtx, handleID).Code)
}

func TestBulkServiceDeleteFailureResult(t *testing.T) {
	manager := asyncbulk.NewManager("SMR-BULK-TEST", time.Minute)
	service := NewBulkService(smBulkServiceStub{
		deleteResult: asyncbulk.OperationResult{
			Success:         false,
			ProcessedCount:  2,
			SuccessfulCount: 0,
			FailedCount:     2,
			Failures: []asyncbulk.ItemFailure{
				{Index: 1, Identifier: "urn:bad", StatusCode: http.StatusNotFound, Message: "not found"},
			},
		},
	}, manager)

	start := service.StartDelete(context.Background(), []string{"urn:ok", "urn:bad"})
	require.Equal(t, http.StatusAccepted, start.Code)

	location := start.Body.(model.Redirect).Location
	handleID := location[strings.LastIndex(location, "/")+1:]
	awaitSMResultAvailability(t, service, handleID)

	result := service.GetResult(context.Background(), handleID)
	require.Equal(t, http.StatusBadRequest, result.Code)

	body := result.Body.(map[string]any)
	require.EqualValues(t, 2, body["processedCount"])
	require.EqualValues(t, 0, body["successfulCount"])
	require.EqualValues(t, 2, body["failedCount"])
}

func awaitSMResultAvailability(t *testing.T, service *BulkService, handleID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := service.GetStatus(context.Background(), handleID)
		if status.Code == http.StatusFound {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("SMR-BULK-TEST-TIMEOUT %s", handleID)
}
