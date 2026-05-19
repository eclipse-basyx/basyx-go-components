package aasregistryapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestGetAsyncBulkStatus_MapsRetryAfterToHeader(t *testing.T) {
	manager := asyncbulk.NewManager("AASR-BULK-TEST", time.Minute)
	service := NewBulkService(aasBulkServiceStub{}, manager)
	handler := NewBulkHTTPHandler(service)

	router := chi.NewRouter()
	handler.RegisterRoutes(router, true)

	handleID, err := manager.Start("anonymous")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/bulk/status/"+handleID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "2", rr.Header().Get("Retry-After"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, "Running", payload["executionState"])
	require.Equal(t, true, payload["success"])
	_, hasRetryAfter := payload["retryAfter"]
	require.False(t, hasRetryAfter, "retryAfter must be present in header, not in response body")
}
