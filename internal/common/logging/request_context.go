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

package logging

import "context"

type requestMetadataKey struct{}

type requestMetadata struct {
	requestID     string
	correlationID string
}

func contextWithRequestMetadata(ctx context.Context, metadata requestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

// RequestIDFromContext returns the request identifier attached by the HTTP
// request middleware, or an empty string when no request metadata is present.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	metadata, _ := ctx.Value(requestMetadataKey{}).(requestMetadata)
	return metadata.requestID
}

// CorrelationIDFromContext returns the correlation identifier attached by the
// HTTP request middleware, or an empty string when no request metadata is present.
func CorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	metadata, _ := ctx.Value(requestMetadataKey{}).(requestMetadata)
	return metadata.correlationID
}
