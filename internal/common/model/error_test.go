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
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var specTimestampPattern = regexp.MustCompile(`^-?(([1-9][0-9][0-9][0-9]+)|(0[0-9][0-9][0-9]))-((0[1-9])|(1[0-2]))-((0[1-9])|([12][0-9])|(3[01]))T(((([01][0-9])|(2[0-3])):[0-5][0-9]:([0-5][0-9])(\.[0-9]+)?)|24:00:00(\.0+)?)(Z|\+00:00|-00:00)$`)

func TestDefaultErrorHandlerWritesStandardizedParsingError(t *testing.T) {
	recorder := httptest.NewRecorder()

	DefaultErrorHandler(recorder, nil, &ParsingError{Err: errors.New("invalid request payload")}, nil)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}

	var result Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode standardized error response: %v", err)
	}
	body := result.Messages
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

func TestNewErrorResponseWritesResultObject(t *testing.T) {
	response := NewErrorResponse(errors.New("not found"), http.StatusNotFound, "TEST", "Get", "Missing")
	recorder := httptest.NewRecorder()
	if err := EncodeJSONResponse(response.Body, &response.Code, recorder); err != nil {
		t.Fatalf("failed to encode error response: %v", err)
	}

	var raw map[string][]map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("expected Result object with messages, got %s", recorder.Body.String())
	}
	messages := raw["messages"]
	if len(raw) != 1 || len(messages) != 1 {
		t.Fatalf("expected exactly one messages entry, got %s", recorder.Body.String())
	}
	if messages[0]["correlationId"] != "TEST-404-Get-NotFound-Missing" {
		t.Fatalf("unexpected correlation ID %q", messages[0]["correlationId"])
	}
	if !specTimestampPattern.MatchString(messages[0]["timestamp"]) {
		t.Fatalf("timestamp %q does not match the AAS API pattern", messages[0]["timestamp"])
	}
}

func TestNewErrorCorrelationIDIsLimitedToSchemaMaximum(t *testing.T) {
	correlationID := NewErrorCorrelationID("TEST", http.StatusBadRequest, strings.Repeat("ä", 200), "Info")

	if length := utf8.RuneCountInString(correlationID); length != maxCorrelationIDLength {
		t.Fatalf("expected %d characters, got %d", maxCorrelationIDLength, length)
	}
	if !utf8.ValidString(correlationID) {
		t.Fatalf("expected valid UTF-8 after truncation, got %q", correlationID)
	}
}

func TestErrorTextReturnsFirstMessageText(t *testing.T) {
	response := NewErrorResponse(errors.New(" invalid submodel "), http.StatusBadRequest, "TEST", "Put", "Invalid")

	if text := ErrorText(response.Body); text != "invalid submodel" {
		t.Fatalf("expected first message text, got %q", text)
	}
	if text := ErrorText(Result{}); text != "" {
		t.Fatalf("expected empty text for empty result, got %q", text)
	}
	if text := ErrorText([]Message{{Text: "legacy"}}); text != "" {
		t.Fatalf("expected empty text for non-Result body, got %q", text)
	}
}
