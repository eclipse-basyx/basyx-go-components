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

package auth

import (
	"context"
	"encoding/json"
)

// WithQueryFilter stores the provided query filter in the context.
func WithQueryFilter(ctx context.Context, queryFilter *QueryFilter) context.Context {
	if queryFilter == nil {
		return ctx
	}
	return context.WithValue(ctx, filterKey, queryFilter)
}

// ContextWithoutQueryFilter masks any inherited query filter in ctx.
func ContextWithoutQueryFilter(ctx context.Context) context.Context {
	return WithoutQueryFilter(ctx)
}

// WithoutQueryFilter returns a child context that keeps request metadata but
// removes row-level ABAC filters for technical checks such as existence probes.
func WithoutQueryFilter(ctx context.Context) context.Context {
	return context.WithValue(ctx, filterKey, struct{}{})
}

// CloneQueryFilter returns a deep copy of the provided query filter.
func CloneQueryFilter(queryFilter *QueryFilter) (*QueryFilter, error) {
	if queryFilter == nil {
		return nil, nil
	}

	b, err := json.Marshal(queryFilter)
	if err != nil {
		return nil, err
	}

	var cloned QueryFilter
	if err := json.Unmarshal(b, &cloned); err != nil {
		return nil, err
	}

	return &cloned, nil
}
