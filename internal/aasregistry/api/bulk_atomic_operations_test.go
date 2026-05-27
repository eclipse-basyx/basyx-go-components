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
	"errors"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestAASBulkCreateErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, aasBulkCreateErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusConflict, aasBulkCreateErrorStatusCode(common.NewErrConflict("conflict")))
	require.Equal(t, http.StatusForbidden, aasBulkCreateErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusNotFound, aasBulkCreateErrorStatusCode(common.NewErrNotFound("missing")))
	require.Equal(t, http.StatusInternalServerError, aasBulkCreateErrorStatusCode(errors.New("unknown")))
}

func TestAASBulkPutErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, aasBulkPutErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusConflict, aasBulkPutErrorStatusCode(common.NewErrConflict("conflict")))
	require.Equal(t, http.StatusForbidden, aasBulkPutErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusNotFound, aasBulkPutErrorStatusCode(common.NewErrNotFound("missing")))
	require.Equal(t, http.StatusInternalServerError, aasBulkPutErrorStatusCode(errors.New("unknown")))
}

func TestAASBulkDeleteErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, aasBulkDeleteErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusForbidden, aasBulkDeleteErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusNotFound, aasBulkDeleteErrorStatusCode(common.NewErrNotFound("missing")))
	require.Equal(t, http.StatusInternalServerError, aasBulkDeleteErrorStatusCode(errors.New("unknown")))
}
