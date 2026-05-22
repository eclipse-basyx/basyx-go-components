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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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

func TestGetAllConceptDescriptionsRejectsInvalidCursorWithStandardErrorBody(t *testing.T) {
	t.Parallel()

	invalidCursor := "%"
	_, expectedDecodeErr := common.DecodeString(invalidCursor)
	require.Error(t, expectedDecodeErr)

	sut := NewConceptDescriptionRepositoryAPIAPIService(nil)
	response, err := sut.GetAllConceptDescriptions(contextWithABACDisabled(t), "", "", "", 1, invalidCursor)

	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, response.Code)

	handlers, ok := response.Body.([]common.ErrorHandler)
	require.True(t, ok)
	require.Len(t, handlers, 1)
	require.Equal(t, "Error", handlers[0].MessageType)
	require.Equal(t, expectedDecodeErr.Error(), handlers[0].Text)
	require.Equal(t, "400", handlers[0].Code)
	require.Equal(t, "CDREPO-400-GetAllConceptDescriptions-BadRequest-BadCursor", handlers[0].CorrelationID)
	require.NotEmpty(t, handlers[0].Timestamp)
}
