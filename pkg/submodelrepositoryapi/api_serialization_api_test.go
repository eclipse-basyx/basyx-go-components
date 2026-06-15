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

package openapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

type serializationServiceStub struct {
	response model.ImplResponse
	err      error
}

func (s serializationServiceStub) GenerateSerializationByIds(_ context.Context, _ []string, _ []string, _ bool) (model.ImplResponse, error) {
	return s.response, s.err
}

func TestSerializationRoutesIncludeContextPath(t *testing.T) {
	t.Parallel()

	ctrl := NewSerializationAPIAPIController(serializationServiceStub{}, "/api/v3")
	routes := ctrl.Routes()

	require.Equal(t, "/api/v3/serialization", routes["GenerateSerializationByIds"].Pattern)
}

func TestGenerateSerializationByIdsReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	ctrl := NewSerializationAPIAPIController(serializationServiceStub{
		response: model.Response(http.StatusNotImplemented, nil),
		err:      errors.New("not-implemented"),
	}, "")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/serialization", nil)

	ctrl.GenerateSerializationByIds(rr, req)

	require.Equal(t, http.StatusNotImplemented, rr.Code)
}
