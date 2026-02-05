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

package digitaltwinregistry

import (
	"context"
	"net/http"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type createdAfterKey struct{}

type createdAfterValue struct {
	value *time.Time
	err   error
}

// CreatedAfterMiddleware parses ?createdAfter=... (RFC3339) and stores it in the request context.
// If parsing fails, the error is stored in context and can be handled by the service.
func CreatedAfterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("createdAfter")
		ctx := r.Context()
		if raw == "" {
			ctx = context.WithValue(ctx, createdAfterKey{}, createdAfterValue{})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			bad := common.NewErrBadRequest("invalid createdAfter (expected RFC3339)")
			resp := common.NewErrorResponse(bad, http.StatusBadRequest, "DTR", "CreatedAfterMiddleware", "createdAfter")
			_ = model.EncodeJSONResponse(resp.Body, &resp.Code, w)
			return
		}
		val := createdAfterValue{value: &parsed}
		ctx = context.WithValue(ctx, createdAfterKey{}, val)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CreatedAfterFromContext returns the parsed createdAfter value if present.
// If the param was invalid, err will be non-nil.
func CreatedAfterFromContext(ctx context.Context) (*time.Time, error) {
	val, ok := ctx.Value(createdAfterKey{}).(createdAfterValue)
	if !ok {
		return nil, nil
	}
	return val.value, val.err
}
