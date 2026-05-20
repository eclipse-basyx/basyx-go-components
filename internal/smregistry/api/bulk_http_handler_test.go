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

package smregistryapi

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

func TestGetBulkAsyncStatus_MapsRetryAfterToHeader(t *testing.T) {
	manager := asyncbulk.NewManager("SMR-BULK-TEST", time.Minute)
	service := NewBulkService(smBulkServiceStub{}, manager)
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
