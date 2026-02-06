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
// Author: Martin Stemmer ( Fraunhofer IESE )

// Package openapi implements Submodel Registry Service API helpers.
package openapi

import (
	"log"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// decodePathParam decodes an URL path component and builds a consistent error response.
func decodePathParam(raw, paramName, operation, errorDetail string) (string, *model.ImplResponse, error) {
	decoded, err := common.DecodeString(raw)
	if err != nil {
		log.Printf("[ERROR] [%s] Error in %s: decode %s=%q: %v", componentName, operation, paramName, raw, err)
		resp := common.NewErrorResponse(
			err, http.StatusBadRequest, componentName, operation, errorDetail,
		)
		return "", &resp, nil
	}
	return decoded, nil, nil
}

// decodeCursor wraps cursor decoding with shared logging + error response.
func decodeCursor(raw, operation string) (string, *model.ImplResponse, error) {
	if raw == "" {
		return "", nil, nil
	}
	return decodePathParam(raw, "cursor", operation, "BadCursor")
}

// pagedResponse builds the common paged envelope used across list endpoints.
func pagedResponse[T any](results T, nextCursor string) model.ImplResponse {
	pm := model.PagedResultPagingMetadata{}
	if nextCursor != "" {
		pm.Cursor = common.EncodeString(nextCursor)
	}

	res := struct {
		PagingMetadata model.PagedResultPagingMetadata `json:"pagingMetadata"`
		Result         T                               `json:"result"`
	}{
		PagingMetadata: pm,
		Result:         results,
	}

	return model.Response(http.StatusOK, res)
}
