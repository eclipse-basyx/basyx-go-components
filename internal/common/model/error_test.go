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

package model

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultErrorHandlerWritesStandardizedParsingError(t *testing.T) {
	recorder := httptest.NewRecorder()

	DefaultErrorHandler(recorder, nil, &ParsingError{Err: errors.New("invalid request payload")}, nil)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}

	var body []ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode standardized error response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected one error entry, got %d", len(body))
	}
	if body[0].MessageType != "Error" || body[0].Code != "400" {
		t.Fatalf("expected standardized error metadata, got %#v", body[0])
	}
	if body[0].Text != "invalid request payload" {
		t.Fatalf("expected error text %q, got %q", "invalid request payload", body[0].Text)
	}
	if body[0].CorrelationID == "" || body[0].Timestamp == "" {
		t.Fatalf("expected correlation ID and timestamp, got %#v", body[0])
	}
}
